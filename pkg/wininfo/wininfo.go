package wininfo

type WinGetter interface {
	// GetActiveWindow returns the current active window information
	GetActiveWindow() (*WinInfo, error)
	// OnActiveWindowChange registers a callback function to be called when the active window changes
	// The callback function will be called with the new active window information
	// The function should return an error if it fails to register the callback
	OnActiveWindowChange(ActiveWindowChangeCallback) error
	// Disconnected returns a channel that is closed when the underlying
	// connection to the window information source is lost.
	// Implementations without a live connection may return a channel that never closes.
	Disconnected() <-chan struct{}
	// Close closes the window info service
	Close()
}

type WinInfo struct {
	Title string
	Class string
}

type ActiveWindowChangeCallback func(*WinInfo)
