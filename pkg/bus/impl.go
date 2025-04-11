package bus

import (
	"fmt"
	"log/slog"
	"sort"

	"github.com/jialeicui/golibevdev"

	"github.com/jialeicui/keyswift/pkg/engine"
	"github.com/jialeicui/keyswift/pkg/keys"
	"github.com/jialeicui/keyswift/pkg/wininfo"
)

// Impl processes events
type Impl struct {
	curFocusWindow *wininfo.WinInfo
	engine         engine.Engine
	windowInfo     wininfo.WinGetter
	out            *golibevdev.UInputDev

	beforeSendKeysPerSession func()
}

// New creates a new bus implementation
func New(script string, windowInfo wininfo.WinGetter, out *golibevdev.UInputDev) (*Impl, error) {
	if script == "" {
		return nil, fmt.Errorf("script is required")
	}

	manager := &Impl{
		windowInfo: windowInfo,
		out:        out,
	}

	e, err := engine.NewQuickJS(script)
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

func (m *Impl) SetBeforeSendKeysPerSession(fn func()) {
	m.beforeSendKeysPerSession = fn
}

// ProcessEvent processes an event through the current bus
func (m *Impl) ProcessEvent(event *Event) (bool, error) {
	if event == nil || event.KeyPress == nil {
		return false, nil
	}

	s := newSession(m, event.KeyPress.Keys, m.beforeSendKeysPerSession)
	err := m.engine.Run(s)
	if err != nil {
		return false, fmt.Errorf("failed to run engine: %w", err)
	}
	return s.Handled(), nil
}

// handleWindowFocus handles window focus change events
func (m *Impl) handleWindowFocus(winInfo *wininfo.WinInfo) {
	m.curFocusWindow = winInfo
}

func (m *Impl) GetActiveWindowClass() string {
	slog.Debug("GetActiveWindowClass", "curFocusWindow", m.curFocusWindow)
	if m.curFocusWindow != nil {
		return m.curFocusWindow.Class
	}
	return ""
}

func (m *Impl) SendKeys(keyCodes []keys.Key) {
	slog.Debug("SendKeys", "keyCodes", keyCodes)

	cloned := append([]keys.Key{}, keyCodes...)
	sort.Slice(cloned, func(i, j int) bool {
		return keys.IsModifier(cloned[i])
	})

	// modifier keys first
	for _, key := range cloned {
		err := m.out.WriteEvent(golibevdev.EvKey, key, 1)
		if err != nil {
			slog.Error("failed to send key event", "error", err)
		}
	}

	// send sync event
	_ = m.out.WriteEvent(golibevdev.EvSyn, golibevdev.SynReport, 0)

	// send release event
	for _, key := range cloned {
		_ = m.out.WriteEvent(golibevdev.EvKey, key, 0)
	}

	// send sync event
	_ = m.out.WriteEvent(golibevdev.EvSyn, golibevdev.SynReport, 0)

	slog.Debug("SendKeys done", "input", keyCodes)
}

func (m *Impl) UpdateWindowMonitor(windowInfo wininfo.WinGetter) {
	m.windowInfo = windowInfo
	if windowInfo != nil {
		err := windowInfo.OnActiveWindowChange(m.handleWindowFocus)
		if err != nil {
			slog.Error("failed to register window focus handler", "error", err)
		}
	}
}
