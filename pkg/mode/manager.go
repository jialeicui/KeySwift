package mode

import (
	"fmt"
	"log/slog"

	"github.com/jialeicui/keyswift/pkg/engine"
	"github.com/jialeicui/keyswift/pkg/keys"
	"github.com/jialeicui/keyswift/pkg/wininfo"
)

// Manager manages the modes and processes events
type Manager struct {
	curFocusWindow *wininfo.WinInfo
	currentKeys    []string
	engine         *engine.Engine
	keyHandler     keys.FunctionKeys
	windowInfo     wininfo.WinGetter
	matched        bool
}

// NewManager creates a new mode manager
func NewManager(script string, keyHandler keys.FunctionKeys, windowInfo wininfo.WinGetter) (*Manager, error) {
	if script == "" {
		return nil, fmt.Errorf("script is required")
	}

	manager := &Manager{
		keyHandler: keyHandler,
		windowInfo: windowInfo,
	}

	e, err := engine.New(manager, script)
	if err != nil {
		return nil, fmt.Errorf("failed to create engine: %w", err)
	}

	manager.engine = e

	// Listen for window focus changes
	if windowInfo != nil {
		err := windowInfo.OnActiveWindowChange(manager.handleWindowFocus)
		if err != nil {
			return nil, fmt.Errorf("failed to register window focus handler: %w", err)
		}
	}

	return manager, nil
}

// ProcessEvent processes an event through the current mode
func (m *Manager) ProcessEvent(event *Event) (bool, error) {
	if event == nil || event.KeyPress == nil {
		return false, nil
	}
	m.currentKeys = event.KeyPress.Keys
	slog.Debug("currentKeys", "keys", m.currentKeys)

	m.matched = false

	err := m.engine.Run()
	if err != nil {
		return false, fmt.Errorf("failed to run engine: %w", err)
	}
	return m.matched, nil
}

// handleWindowFocus handles window focus change events
func (m *Manager) handleWindowFocus(winInfo *wininfo.WinInfo) {
	m.curFocusWindow = winInfo
}

func (m *Manager) GetActiveWindowClass() string {
	if m.curFocusWindow != nil {
		return m.curFocusWindow.Class
	}
	return ""
}

func (m *Manager) GetPressedKeys() []string {
	return m.currentKeys
}

func (m *Manager) GetKeyState(key string) string {
	//TODO implement me
	panic("implement me")
}

func (m *Manager) SendKeys(keys []string) {
	m.matched = true
}
