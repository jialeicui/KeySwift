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