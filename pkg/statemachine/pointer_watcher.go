package statemachine

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/jialeicui/golibevdev"
)

const (
	inputDirPath   = "/dev/input"
	pointerRetryMs = 500

	eviocgid     = 0x80084502 // _IOC(_IOC_READ, 'E', 0x02, sizeof(struct input_id))
	keyBitmapLen = 96         // KEY_MAX (0x2ff) rounded up to bytes
	relBitmapLen = 2          // REL_MAX (0x0f) rounded up to bytes
)

var pointerButtonCodes = []uint16{
	uint16(golibevdev.BtnLeft),
	uint16(golibevdev.BtnRight),
	uint16(golibevdev.BtnMiddle),
	uint16(golibevdev.BtnSide),
	uint16(golibevdev.BtnExtra),
	uint16(golibevdev.BtnForward),
	uint16(golibevdev.BtnBack),
	uint16(golibevdev.BtnTask),
	uint16(golibevdev.BtnTouch),
	uint16(golibevdev.BtnStylus),
	uint16(golibevdev.BtnStylus2),
}

// pointerWatcher observes pointer button presses and wheel scrolls on devices
// KeySwift does not grab, without grabbing them, so absorbed modifiers (e.g.
// Super/Cmd or Alt) can be flushed when the user clicks or scrolls.
type pointerWatcher struct {
	inputDir string
	excluded map[string]struct{}
	onAction func()
	openFile func(path string) (*os.File, error)
	hasCaps  func(file *os.File) bool
}

func newPointerWatcher(inputDir string, excluded map[string]struct{}, onAction func()) *pointerWatcher {
	return &pointerWatcher{
		inputDir: inputDir,
		excluded: excluded,
		onAction: onAction,
		openFile: func(path string) (*os.File, error) {
			return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
		},
		hasCaps: hasPointerCapabilities,
	}
}

func (w *pointerWatcher) run(ctx context.Context) {
	epfd, err := syscall.EpollCreate1(syscall.EPOLL_CLOEXEC)
	if err != nil {
		slog.Warn("Failed to create epoll for pointer watcher", "error", err)
		return
	}
	defer syscall.Close(epfd)

	pipeFDs := []int{-1, -1}
	if err := syscall.Pipe2(pipeFDs, syscall.O_NONBLOCK|syscall.O_CLOEXEC); err != nil {
		slog.Warn("Failed to create wake pipe for pointer watcher", "error", err)
		return
	}
	defer syscall.Close(pipeFDs[0])
	defer syscall.Close(pipeFDs[1])

	if err := epollAdd(epfd, pipeFDs[0]); err != nil {
		slog.Warn("Failed to watch wake pipe", "error", err)
		return
	}

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_, _ = syscall.Write(pipeFDs[1], []byte{1})
		case <-done:
		}
	}()

	inotifyFD := -1
	if fd, err := syscall.InotifyInit1(syscall.IN_NONBLOCK | syscall.IN_CLOEXEC); err == nil {
		if _, err := syscall.InotifyAddWatch(fd, w.inputDir, syscall.IN_CREATE); err == nil {
			if err := epollAdd(epfd, fd); err == nil {
				inotifyFD = fd
			}
		}
		if inotifyFD == -1 {
			syscall.Close(fd)
		}
	}
	if inotifyFD != -1 {
		defer syscall.Close(inotifyFD)
	}

	devices := make(map[int]*os.File)
	defer func() {
		for _, file := range devices {
			file.Close()
		}
	}()

	retry := w.syncDevices(epfd, devices)
	rescan := false
	events := make([]syscall.EpollEvent, 16)
	buf := make([]byte, inputEventSize*32)
	inotifyBuf := make([]byte, 4096)

	for {
		if rescan {
			retry = w.syncDevices(epfd, devices)
			rescan = false
		}

		timeout := -1
		if retry {
			timeout = pointerRetryMs
		}

		n, err := syscall.EpollWait(epfd, events, timeout)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			slog.Warn("Pointer watcher epoll failed", "error", err)
			return
		}
		if ctx.Err() != nil {
			return
		}
		if n == 0 {
			rescan = true
			continue
		}

		for i := 0; i < n; i++ {
			fd := int(events[i].Fd)
			switch fd {
			case pipeFDs[0]:
				return
			case inotifyFD:
				drainFD(inotifyFD, inotifyBuf)
				rescan = true
			default:
				file, ok := devices[fd]
				if !ok {
					continue
				}
				if !w.readDevice(file, buf) {
					_ = syscall.EpollCtl(epfd, syscall.EPOLL_CTL_DEL, fd, nil)
					file.Close()
					delete(devices, fd)
					rescan = true
				}
			}
		}
	}
}

func (w *pointerWatcher) syncDevices(epfd int, devices map[int]*os.File) (retry bool) {
	entries, err := os.ReadDir(w.inputDir)
	if err != nil {
		return true
	}

	openPaths := make(map[string]struct{}, len(devices))
	for _, file := range devices {
		openPaths[file.Name()] = struct{}{}
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "event") {
			continue
		}
		path := filepath.Join(w.inputDir, entry.Name())
		if _, skip := w.excluded[path]; skip {
			continue
		}
		if _, ok := openPaths[path]; ok {
			continue
		}

		file, err := w.openFile(path)
		if err != nil {
			// devtmpfs creates event nodes as root:root 0600 before udev applies
			// group permissions, so retry transient permission/open failures.
			if !errors.Is(err, os.ErrNotExist) {
				retry = true
			}
			continue
		}
		if !w.hasCaps(file) {
			file.Close()
			continue
		}
		if err := epollAdd(epfd, int(file.Fd())); err != nil {
			file.Close()
			continue
		}
		devices[int(file.Fd())] = file
		slog.Debug("Watching pointer device", "path", path)
	}
	return retry
}

func (w *pointerWatcher) readDevice(file *os.File, buf []byte) bool {
	for {
		n, err := syscall.Read(int(file.Fd()), buf)
		if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
			return true
		}
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				slog.Debug("Pointer device disconnected", "path", file.Name(), "error", err)
			}
			return false
		}
		if n == 0 {
			return false
		}

		for offset := 0; offset+inputEventSize <= n; offset += inputEventSize {
			ev := buf[offset : offset+inputEventSize]
			typ := binary.LittleEndian.Uint16(ev[16:18])
			code := binary.LittleEndian.Uint16(ev[18:20])
			value := int32(binary.LittleEndian.Uint32(ev[20:24]))
			if isPointerActionEvent(typ, code, value) && w.onAction != nil {
				w.onAction()
			}
		}
		if n < len(buf) {
			return true
		}
	}
}

func isPointerActionEvent(typ, code uint16, value int32) bool {
	switch typ {
	case evKey:
		if value != 1 {
			return false
		}
		for _, btn := range pointerButtonCodes {
			if btn == code {
				return true
			}
		}
	case evRel:
		if value == 0 {
			return false
		}
		switch golibevdev.RelativeEventCode(code) {
		case golibevdev.RelWheel, golibevdev.RelHWheel, golibevdev.RelWheelHiRes, golibevdev.RelHWheelHiRes:
			return true
		}
	}
	return false
}

func hasPointerCapabilities(file *os.File) bool {
	fd := file.Fd()

	var id [8]byte
	if err := inputIoctl(fd, eviocgid, uintptr(unsafe.Pointer(&id[0]))); err == nil {
		if binary.LittleEndian.Uint16(id[0:2]) == busVirtual {
			return false
		}
	}

	var keyBits [keyBitmapLen]byte
	if err := inputIoctl(fd, eviocgbit(evKey, len(keyBits)), uintptr(unsafe.Pointer(&keyBits[0]))); err == nil {
		for _, code := range pointerButtonCodes {
			if bitmapHasBit(keyBits[:], code) {
				return true
			}
		}
	}

	var relBits [relBitmapLen]byte
	if err := inputIoctl(fd, eviocgbit(evRel, len(relBits)), uintptr(unsafe.Pointer(&relBits[0]))); err == nil {
		if bitmapHasBit(relBits[:], uint16(golibevdev.RelWheel)) {
			return true
		}
	}

	return false
}

func eviocgbit(evType, length int) uint {
	return uint((2 << 30) | (length << 16) | (int('E') << 8) | (0x20 + evType))
}

func bitmapHasBit(bitmap []byte, code uint16) bool {
	idx := int(code / 8)
	if idx >= len(bitmap) {
		return false
	}
	return bitmap[idx]&(1<<(code%8)) != 0
}

func epollAdd(epfd, fd int) error {
	event := syscall.EpollEvent{Events: syscall.EPOLLIN, Fd: int32(fd)}
	return syscall.EpollCtl(epfd, syscall.EPOLL_CTL_ADD, fd, &event)
}

func drainFD(fd int, buf []byte) {
	for {
		if _, err := syscall.Read(fd, buf); err != nil {
			return
		}
	}
}
