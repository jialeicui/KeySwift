package mode

import (
	"fmt"
	"log/slog"

	"github.com/expr-lang/expr"
	"github.com/jialeicui/golibevdev"

	"github.com/jialeicui/keyswift/pkg/conf"
	"github.com/jialeicui/keyswift/pkg/keys"
	"github.com/jialeicui/keyswift/pkg/wininfo"
)

// ActionHandler defines an interface for handling different types of actions
type ActionHandler interface {
	Handle(manager *Manager, action *conf.Action) error
}

// ActionHandlerImpl handles mode transitions
type ActionHandlerImpl struct{}

func (h *ActionHandlerImpl) Handle(manager *Manager, action *conf.Action) error {
	if action.MarkMode == nil {
		return nil
	}

	for modeName, operation := range *action.MarkMode {
		manager.env.Set(modeName, operation)
	}

	return nil
}

// KeyMapActionHandler handles mapping keys to other keys
type KeyMapActionHandler struct{}

func (h *KeyMapActionHandler) Handle(manager *Manager, action *conf.Action) error {
	if action.MapToKeys == nil {
		return nil
	}

	// Send the mapped keys through the keyHandler
	if err := manager.keyHandler.SendKeys(configKeysToKeys(action.MapToKeys.Keys)); err != nil {
		return fmt.Errorf("failed to send keys: %w", err)
	}

	return nil
}

func configKeysToKeys(configKeys []conf.Key) []golibevdev.KeyEventCode {
	ret := make([]golibevdev.KeyEventCode, 0, len(configKeys))
	for _, configKey := range configKeys {
		ret = append(ret, keyStringToKeyCode(configKey.Key))
		for _, m := range configKey.Modifiers {
			ret = append(ret, keyStringToKeyCode(m))
		}
	}
	return ret
}

func keyStringToKeyCode(key string) golibevdev.KeyEventCode {
	// This is a simplified version - you'd want a complete mapping
	switch key {
	case "a":
		return golibevdev.KeyA
	case "b":
		return golibevdev.KeyB
	case "down":
		return golibevdev.KeyDown
	case "up":
		return golibevdev.KeyUp
	default:
		return golibevdev.KeyReserved
	}
}

// TriggerActionHandler handles triggers like commands or built-in actions
type TriggerActionHandler struct{}

func (h *TriggerActionHandler) Handle(manager *Manager, action *conf.Action) error {
	if action.Trigger == nil {
		return nil
	}

	// Log the trigger for now
	slog.Info("Trigger received", "trigger", action.Trigger)
	// TODO: Implement trigger execution
	return nil
}

// Manager manages the modes and processes events
type Manager struct {
	env            *Env
	config         *conf.Config
	modeMap        map[Expr]Mode
	keyHandler     keys.FunctionKeys
	windowInfo     wininfo.WinGetter
	defaultMode    string
	actionHandlers []ActionHandler
}

// NewManager creates a new mode manager
func NewManager(config *conf.Config, keyHandler keys.FunctionKeys, windowInfo wininfo.WinGetter) (*Manager, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	manager := &Manager{
		env:         NewEnv(),
		config:      config,
		modeMap:     make(map[Expr]Mode),
		keyHandler:  keyHandler,
		windowInfo:  windowInfo,
		defaultMode: "default", // Default mode name
		actionHandlers: []ActionHandler{
			&ActionHandlerImpl{},
			&KeyMapActionHandler{},
			&TriggerActionHandler{},
		},
	}

	// Initialize modes from config
	if err := manager.initModes(); err != nil {
		return nil, err
	}

	// Listen for window focus changes
	if windowInfo != nil {
		err := windowInfo.OnActiveWindowChange(manager.handleWindowFocus)
		if err != nil {
			return nil, fmt.Errorf("failed to register window focus handler: %w", err)
		}
	}

	return manager, nil
}

// initModes initializes modes from config
func (m *Manager) initModes() error {
	for name, actions := range m.config.ModeActions {
		mode := NewConfigMode(name, actions)
		var e Expr
		if actions.If == "" {
			e = NewTrueExpr()
		} else {
			p, err := expr.Compile(actions.If)
			if err != nil {
				return fmt.Errorf("failed to compile expression for mode '%s': %w", name, err)
			}
			e = &ExprImpl{p: p}
		}
		m.modeMap[e] = mode
	}
	return nil
}

// ProcessEvent processes an event through the current mode
func (m *Manager) ProcessEvent(event *Event) (bool, error) {
	var handled bool
	for e, mode := range m.modeMap {
		slog.Debug("Processing mode", "mode", mode.Name(), "env", m.env.data)
		if e.Test(m.env) {
			slog.Debug("Matched mode", "mode", mode.Name())
			actions := mode.ProcessEvent(event)
			if actions != nil {
				handled = true
				for _, action := range actions {
					if err := m.executeAction(action); err != nil {
						return false, err
					}
				}
			}
		}
		slog.Debug("------")
	}

	return handled, nil
}

// executeAction executes an action
func (m *Manager) executeAction(action *conf.Action) error {
	// Execute the action using each handler
	for _, handler := range m.actionHandlers {
		if err := handler.Handle(m, action); err != nil {
			return err
		}
	}
	return nil
}

// handleWindowFocus handles window focus change events
func (m *Manager) handleWindowFocus(winInfo *wininfo.WinInfo) {
	event := &Event{
		WindowFocus: &WindowFocusEvent{
			Window: winInfo,
		},
	}
	slog.Debug("Window focus event", "window", event.WindowFocus.Window)
	if _, err := m.ProcessEvent(event); err != nil {
		slog.Error("Error processing window focus event", "error", err)
	}
}
