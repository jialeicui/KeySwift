package evdev

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOverview(t *testing.T) {
	var (
		must = require.New(t)
	)

	overview := &OverviewImpl{}
	devices, err := overview.ListInputDevices()
	must.NoError(err)
	must.NotEmpty(devices)

	for _, device := range devices {
		must.NotEmpty(device.Name)
		must.NotEmpty(device.Path)
		t.Logf("Name: %s, Path: %s", device.Name, device.Path)
	}
}

func TestFindDeviceByName(t *testing.T) {
	devices := []*InputDevice{
		{Name: "Keyboard", Path: "/dev/input/event1"},
		{Name: "Keyboard", Path: "/dev/input/event2"},
		{Name: "Other", Path: "/dev/input/event3"},
	}

	selected := FindDeviceByName(devices, "Keyboard", map[string]struct{}{
		"/dev/input/event1": {},
	})
	require.NotNil(t, selected)
	require.Equal(t, "/dev/input/event2", selected.Path)

	missing := FindDeviceByName(devices, "Missing", nil)
	require.Nil(t, missing)
}
