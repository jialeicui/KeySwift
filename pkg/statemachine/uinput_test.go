package statemachine

import (
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jialeicui/golibevdev"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVirtualPointerAxesCoverMotionAndScroll(t *testing.T) {
	for _, axis := range []golibevdev.RelativeEventCode{
		golibevdev.RelX,
		golibevdev.RelY,
		golibevdev.RelWheel,
		golibevdev.RelHWheel,
		golibevdev.RelWheelHiRes,
		golibevdev.RelHWheelHiRes,
	} {
		assert.Contains(t, virtualPointerAxes, uint16(axis), "axis 0x%x must be enabled", uint16(axis))
	}
}

func TestUinputWriteRawEventMarshalsInputEvent(t *testing.T) {
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	defer reader.Close()

	device := &uinputLowLevelDevice{file: writer}
	require.NoError(t, device.WriteEvent(golibevdev.EvRel, golibevdev.RelX, -7))
	require.NoError(t, device.Sync())
	require.NoError(t, writer.Close())

	raw, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Len(t, raw, 2*inputEventSize)

	first := raw[:inputEventSize]
	assert.Equal(t, uint64(0), binary.LittleEndian.Uint64(first[0:8]), "timestamp must be zeroed")
	assert.Equal(t, uint16(golibevdev.EvRel), binary.LittleEndian.Uint16(first[16:18]))
	assert.Equal(t, uint16(golibevdev.RelX), binary.LittleEndian.Uint16(first[18:20]))
	assert.Equal(t, int32(-7), int32(binary.LittleEndian.Uint32(first[20:24])))

	second := raw[inputEventSize:]
	assert.Equal(t, uint16(evSyn), binary.LittleEndian.Uint16(second[16:18]))
	assert.Equal(t, uint16(synReport), binary.LittleEndian.Uint16(second[18:20]))
	assert.Equal(t, int32(0), int32(binary.LittleEndian.Uint32(second[20:24])))
}

func TestUinputLowLevelDeviceLifecycle(t *testing.T) {
	if _, err := os.Stat(uinputDevicePath); err != nil {
		t.Skipf("uinput is not available: %v", err)
	}

	const name = "keyswift-uinput-test"
	device, err := newUinputLowLevelDevice(name)
	if err != nil {
		t.Skipf("cannot create uinput device, insufficient permissions: %v", err)
	}

	// SYN_REPORT is harmless to inject even on a live desktop.
	require.NoError(t, device.Sync())

	relCaps := waitForRelCapabilities(t, name)
	for _, axis := range virtualPointerAxes {
		assert.True(t, relCaps&(1<<axis) != 0, "relative axis 0x%x not enabled on the virtual device", axis)
	}

	require.NoError(t, device.Close())
}

// waitForRelCapabilities finds the virtual device by name in sysfs and
// returns its EV_REL capability bitmap.
func waitForRelCapabilities(t *testing.T, name string) uint64 {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		matches, _ := filepath.Glob("/sys/devices/virtual/input/input*/name")
		for _, match := range matches {
			content, err := os.ReadFile(match)
			if err != nil || strings.TrimSpace(string(content)) != name {
				continue
			}

			raw, err := os.ReadFile(filepath.Join(filepath.Dir(match), "capabilities", "rel"))
			if err != nil {
				t.Fatalf("read rel capabilities of %q: %v", name, err)
			}
			bitmap, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 16, 64)
			if err != nil {
				t.Fatalf("parse rel capabilities of %q: %v", name, err)
			}
			return bitmap
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("virtual device %q did not appear in sysfs", name)
	return 0
}
