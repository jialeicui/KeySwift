package evdev

type Overview interface {
	ListInputDevices() ([]*InputDevice, error)
}

type InputDevice struct {
	Name string
	Path string
}