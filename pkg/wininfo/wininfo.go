package wininfo

type WinGetter interface {
	// GetActiveWindow returns the current active window information
	GetActiveWindow() (*WinInfo, error)
	// OnActiveWindowChange registers a callback function to be called when the active window changes
	// The callback function will be called with the new active window information
	// The function should return an error if it fails to register the callback
	OnActiveWindowChange(ActiveWindowChangeCallback) error
}

type WinInfo struct {
	Title string
	Class string
}

type ActiveWindowChangeCallback func(*WinInfo)