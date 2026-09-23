package statemachine

import (
	"fmt"
	"log/slog"
	"os"
	"syscall"
	"unsafe"

	"github.com/jialeicui/golibevdev"
)

// uinput ioctl requests, see linux/uinput.h.
const (
	uiSetEvBit   = 0x40045564
	uiSetKeyBit  = 0x40045565
	uiSetRelBit  = 0x40045566
	uiSetMscBit  = 0x40045568
	uiDevCreate  = 0x00005501
	uiDevDestroy = 0x00005502
)

const (
	uinputDevicePath  = "/dev/uinput"
	uinputMaxNameLen  = 80
	uinputUserDevSize = 1116 // sizeof(struct uinput_user_dev)
	inputEventSize    = 24   // sizeof(struct input_event) on 64-bit Linux

	evSyn = 0x00
	evKey = 0x01
	evRel = 0x02
	evMsc = 0x04

	synReport = 0x00
	mscScan   = 0x04

	keyMax     = 0x2ff
	busVirtual = 0x06
)

// virtualPointerAxes are the relative axes the virtual device advertises so
// forwarded pointer events (motion, vertical/horizontal and hi-res scroll)
// are not silently dropped by the kernel.
var virtualPointerAxes = []uint16{
	uint16(golibevdev.RelX),
	uint16(golibevdev.RelY),
	uint16(golibevdev.RelHWheel),
	uint16(golibevdev.RelDial),
	uint16(golibevdev.RelWheel),
	uint16(golibevdev.RelMisc),
	uint16(golibevdev.RelWheelHiRes),
	uint16(golibevdev.RelHWheelHiRes),
}

// inputEvent mirrors struct input_event on 64-bit Linux.
type inputEvent struct {
	Sec   int64
	Usec  int64
	Type  uint16
	Code  uint16
	Value int32
}

// uinputUserDev mirrors struct uinput_user_dev (legacy setup interface).
type uinputUserDev struct {
	Name         [uinputMaxNameLen]byte
	BusType      uint16
	Vendor       uint16
	Product      uint16
	Version      uint16
	FFEffectsMax uint32
	AbsMax       [64]int32
	AbsMin       [64]int32
	AbsFuzz      [64]int32
	AbsFlat      [64]int32
}

// uinputLowLevelDevice is a virtual input device created through the kernel
// uinput interface directly. golibevdev only builds single-purpose virtual
// devices (keyboard-only, mouse-only), while KeySwift needs a single device
// that emits both remapped key events and forwarded pointer events.
type uinputLowLevelDevice struct {
	file *os.File
}

func newUinputLowLevelDevice(name string) (*uinputLowLevelDevice, error) {
	if unsafe.Sizeof(inputEvent{}) != inputEventSize || unsafe.Sizeof(uinputUserDev{}) != uinputUserDevSize {
		return nil, fmt.Errorf("unsupported platform: unexpected uinput struct layout")
	}
	if len(name) >= uinputMaxNameLen {
		return nil, fmt.Errorf("device name %q exceeds %d bytes", name, uinputMaxNameLen-1)
	}

	file, err := os.OpenFile(uinputDevicePath, os.O_WRONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", uinputDevicePath, err)
	}

	device := &uinputLowLevelDevice{file: file}
	if err := device.setup(name); err != nil {
		file.Close()
		return nil, err
	}
	return device, nil
}

func (d *uinputLowLevelDevice) setup(name string) error {
	fd := d.file.Fd()

	for _, typ := range []uint16{evKey, evRel, evMsc} {
		if err := uinputIoctl(fd, uiSetEvBit, uintptr(typ)); err != nil {
			return fmt.Errorf("enable event type 0x%x: %w", typ, err)
		}
	}
	for code := uint16(0); code <= keyMax; code++ {
		if err := uinputIoctl(fd, uiSetKeyBit, uintptr(code)); err != nil {
			return fmt.Errorf("enable key code 0x%x: %w", code, err)
		}
	}
	for _, axis := range virtualPointerAxes {
		if err := uinputIoctl(fd, uiSetRelBit, uintptr(axis)); err != nil {
			return fmt.Errorf("enable relative axis 0x%x: %w", axis, err)
		}
	}
	if err := uinputIoctl(fd, uiSetMscBit, mscScan); err != nil {
		return fmt.Errorf("enable msc scan code: %w", err)
	}

	var request uinputUserDev
	copy(request.Name[:], name)
	request.BusType = busVirtual
	request.Vendor = 0x1
	request.Product = 0x1
	request.Version = 0x1

	setup := unsafe.Slice((*byte)(unsafe.Pointer(&request)), unsafe.Sizeof(request))
	if _, err := d.file.Write(setup); err != nil {
		return fmt.Errorf("write uinput device setup: %w", err)
	}
	if err := uinputIoctl(fd, uiDevCreate, 0); err != nil {
		return fmt.Errorf("create uinput device: %w", err)
	}
	return nil
}

func (d *uinputLowLevelDevice) WriteKey(key KeyCode, value int32) error {
	return d.writeRawEvent(evKey, uint16(key), value)
}

func (d *uinputLowLevelDevice) WriteEvent(typ golibevdev.EventType, code golibevdev.EventCode, value int32) error {
	return d.writeRawEvent(uint16(typ), code.Value(), value)
}

func (d *uinputLowLevelDevice) Sync() error {
	return d.writeRawEvent(evSyn, synReport, 0)
}

func (d *uinputLowLevelDevice) Close() error {
	if err := uinputIoctl(d.file.Fd(), uiDevDestroy, 0); err != nil {
		slog.Warn("Failed to destroy uinput device", "error", err)
	}
	return d.file.Close()
}

// writeRawEvent marshals and writes one struct input_event. The timestamp is
// left zeroed, matching libevdev_uinput_write_event behavior.
func (d *uinputLowLevelDevice) writeRawEvent(typ, code uint16, value int32) error {
	ev := inputEvent{Type: typ, Code: code, Value: value}
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&ev)), unsafe.Sizeof(ev))

	for len(buf) > 0 {
		n, err := d.file.Write(buf)
		if err != nil {
			return fmt.Errorf("write input event: %w", err)
		}
		buf = buf[n:]
	}
	return nil
}

func uinputIoctl(fd uintptr, request uint, arg uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(request), arg)
	if errno != 0 {
		return fmt.Errorf("uinput ioctl 0x%x: %w", request, errno)
	}
	return nil
}
