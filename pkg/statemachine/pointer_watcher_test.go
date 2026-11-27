package statemachine

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/jialeicui/golibevdev"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func marshalInputEvent(typ, code uint16, value int32) []byte {
	ev := inputEvent{Type: typ, Code: code, Value: value}
	raw := unsafe.Slice((*byte)(unsafe.Pointer(&ev)), unsafe.Sizeof(ev))
	return append([]byte(nil), raw...)
}

func TestIsPointerActionEvent(t *testing.T) {
	tests := []struct {
		name  string
		typ   uint16
		code  uint16
		value int32
		want  bool
	}{
		{name: "left button press", typ: evKey, code: uint16(golibevdev.BtnLeft), value: 1, want: true},
		{name: "right button press", typ: evKey, code: uint16(golibevdev.BtnRight), value: 1, want: true},
		{name: "touchpad tap", typ: evKey, code: uint16(golibevdev.BtnTouch), value: 1, want: true},
		{name: "button release ignored", typ: evKey, code: uint16(golibevdev.BtnLeft), value: 0, want: false},
		{name: "keyboard key ignored", typ: evKey, code: uint16(golibevdev.KeyA), value: 1, want: false},
		{name: "wheel scroll", typ: evRel, code: uint16(golibevdev.RelWheel), value: -1, want: true},
		{name: "hi-res wheel scroll", typ: evRel, code: uint16(golibevdev.RelWheelHiRes), value: 15, want: true},
		{name: "horizontal wheel scroll", typ: evRel, code: uint16(golibevdev.RelHWheel), value: 1, want: true},
		{name: "zero wheel ignored", typ: evRel, code: uint16(golibevdev.RelWheel), value: 0, want: false},
		{name: "pointer motion ignored", typ: evRel, code: uint16(golibevdev.RelX), value: 10, want: false},
		{name: "syn report ignored", typ: evSyn, code: uint16(synReport), value: 0, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isPointerActionEvent(tt.typ, tt.code, tt.value))
		})
	}
}

func TestBitmapHasBit(t *testing.T) {
	bitmap := make([]byte, keyBitmapLen)
	bitmap[uint16(golibevdev.BtnLeft)/8] = 1 << (uint16(golibevdev.BtnLeft) % 8)

	assert.True(t, bitmapHasBit(bitmap, uint16(golibevdev.BtnLeft)))
	assert.False(t, bitmapHasBit(bitmap, uint16(golibevdev.BtnRight)))
	assert.False(t, bitmapHasBit(bitmap, 0x500), "out-of-range code must not panic")
}

func TestEviocgbitEncoding(t *testing.T) {
	assert.Equal(t, uint(0x80084520), eviocgbit(0, 8))
	assert.Equal(t, uint(0x80604521), eviocgbit(evKey, keyBitmapLen))
}

func TestPointerWatcherReadDevice(t *testing.T) {
	pipeFDs := []int{-1, -1}
	require.NoError(t, syscall.Pipe2(pipeFDs, syscall.O_NONBLOCK|syscall.O_CLOEXEC))
	reader := os.NewFile(uintptr(pipeFDs[0]), "event-test")
	defer reader.Close()
	writer := os.NewFile(uintptr(pipeFDs[1]), "event-test-w")
	defer writer.Close()

	var mu sync.Mutex
	count := 0
	w := &pointerWatcher{onAction: func() {
		mu.Lock()
		count++
		mu.Unlock()
	}}

	var stream []byte
	stream = append(stream, marshalInputEvent(evRel, uint16(golibevdev.RelX), 5)...)
	stream = append(stream, marshalInputEvent(evKey, uint16(golibevdev.BtnLeft), 1)...)
	stream = append(stream, marshalInputEvent(evSyn, synReport, 0)...)
	stream = append(stream, marshalInputEvent(evKey, uint16(golibevdev.BtnLeft), 0)...)
	stream = append(stream, marshalInputEvent(evSyn, synReport, 0)...)
	stream = append(stream, marshalInputEvent(evRel, uint16(golibevdev.RelWheelHiRes), -15)...)
	_, err := writer.Write(stream)
	require.NoError(t, err)

	buf := make([]byte, inputEventSize*32)
	assert.True(t, w.readDevice(reader, buf), "empty non-blocking fd must return true on EAGAIN")
	mu.Lock()
	assert.Equal(t, 2, count)
	mu.Unlock()
}

func TestPointerWatcherRunLifecycleAndHotplug(t *testing.T) {
	dir := t.TempDir()
	excludedPath := filepath.Join(dir, "event9")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "event0"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ignored"), nil, 0o600))
	require.NoError(t, os.WriteFile(excludedPath, nil, 0o600))

	type testPipe struct {
		writer *os.File
	}
	var mu sync.Mutex
	writers := make(map[string]*os.File)
	opened := make(chan string, 8)
	actions := make(chan struct{}, 8)

	w := newPointerWatcher(dir, map[string]struct{}{excludedPath: {}}, func() {
		actions <- struct{}{}
	})
	w.hasCaps = func(*os.File) bool { return true }
	w.openFile = func(path string) (*os.File, error) {
		pipeFDs := []int{-1, -1}
		if err := syscall.Pipe2(pipeFDs, syscall.O_NONBLOCK|syscall.O_CLOEXEC); err != nil {
			return nil, err
		}
		reader := os.NewFile(uintptr(pipeFDs[0]), path)
		writer := os.NewFile(uintptr(pipeFDs[1]), path+"-w")
		mu.Lock()
		writers[path] = writer
		mu.Unlock()
		opened <- path
		return reader, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.run(ctx)
	}()

	select {
	case path := <-opened:
		assert.Equal(t, filepath.Join(dir, "event0"), path)
	case <-time.After(2 * time.Second):
		t.Fatal("initial pointer device was not opened")
	}

	sendEvent := func(path string, raw []byte) {
		t.Helper()
		mu.Lock()
		writer := writers[path]
		mu.Unlock()
		require.NotNil(t, writer)
		_, err := writer.Write(raw)
		require.NoError(t, err)
	}

	sendEvent(filepath.Join(dir, "event0"), marshalInputEvent(evKey, uint16(golibevdev.BtnTouch), 1))
	select {
	case <-actions:
	case <-time.After(2 * time.Second):
		t.Fatal("pointer action was not delivered")
	}

	hotplugPath := filepath.Join(dir, "event1")
	require.NoError(t, os.WriteFile(hotplugPath, nil, 0o600))
	select {
	case path := <-opened:
		assert.Equal(t, hotplugPath, path)
	case <-time.After(2 * time.Second):
		t.Fatal("hotplugged pointer device was not opened via inotify")
	}

	sendEvent(hotplugPath, marshalInputEvent(evRel, uint16(golibevdev.RelWheel), 1))
	select {
	case <-actions:
	case <-time.After(2 * time.Second):
		t.Fatal("hotplugged pointer action was not delivered")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pointer watcher did not stop on context cancellation")
	}

	mu.Lock()
	for _, writer := range writers {
		writer.Close()
	}
	mu.Unlock()
}

func TestHasPointerCapabilitiesOnLiveDevices(t *testing.T) {
	matches, err := filepath.Glob("/dev/input/event*")
	if err != nil || len(matches) == 0 {
		t.Skip("no evdev devices available")
	}

	checked := false
	for _, path := range matches {
		file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
		if err != nil {
			continue
		}
		t.Logf("device %s hasPointerCapabilities=%v", path, hasPointerCapabilities(file))
		file.Close()
		checked = true
	}
	if !checked {
		t.Skip("no readable evdev devices")
	}
}

func TestMarshalInputEventMatchesKernelLayout(t *testing.T) {
	raw := marshalInputEvent(evKey, uint16(golibevdev.BtnLeft), 1)
	require.Len(t, raw, inputEventSize)
	assert.Equal(t, uint16(evKey), binary.LittleEndian.Uint16(raw[16:18]))
	assert.Equal(t, uint16(golibevdev.BtnLeft), binary.LittleEndian.Uint16(raw[18:20]))
	assert.Equal(t, uint32(1), binary.LittleEndian.Uint32(raw[20:24]))
}
