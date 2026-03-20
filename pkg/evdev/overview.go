package evdev

import (
	"os"
	"strings"

	"github.com/jialeicui/golibevdev"
)

type OverviewImpl struct {
}

func (o *OverviewImpl) ListInputDevices() ([]*InputDevice, error) {
	var ret []*InputDevice
	devices, err := os.ReadDir("/dev/input")

	if err != nil {
		return nil, err
	}

	for _, device := range devices {
		if device.IsDir() {
			continue
		}

		if !strings.HasPrefix(device.Name(), "event") {
			continue
		}

		path := "/dev/input/" + device.Name()
		dev, err := golibevdev.NewInputDev(path)
		if err != nil {
			// Skip devices we can't open (e.g., permission denied)
			continue
		}
		name := dev.Name()
		dev.Close()
		inputDevice := &InputDevice{
			Name: name,
			Path: path,
		}
		ret = append(ret, inputDevice)
	}

	return ret, nil
}

func NewOverviewImpl() *OverviewImpl {
	return &OverviewImpl{}
}

// FindDeviceByName returns the first device with an exact name match that is not excluded.
func FindDeviceByName(devices []*InputDevice, name string, excludedPaths map[string]struct{}) *InputDevice {
	for _, device := range devices {
		if device.Name != name {
			continue
		}
		if _, excluded := excludedPaths[device.Path]; excluded {
			continue
		}
		return device
	}
	return nil
}
