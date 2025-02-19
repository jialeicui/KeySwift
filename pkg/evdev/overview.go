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
			return nil, err
		}
		dev.Close()
		inputDevice := &InputDevice{
			Name: dev.Name(),
			Path: path,
		}
		ret = append(ret, inputDevice)
	}

	return ret, nil
}

func NewOverviewImpl() *OverviewImpl {
	return &OverviewImpl{}
}
