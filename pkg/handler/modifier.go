package handler

import (
	"github.com/jialeicui/golibevdev"

	"github.com/jialeicui/keyswift/pkg/keys"
)

var (
	passThroughKeys = map[golibevdev.KeyEventCode]struct{}{
		golibevdev.KeyLeftCtrl:  {},
		golibevdev.KeyRightCtrl: {},
		golibevdev.KeyLeftAlt:   {},
		golibevdev.KeyRightAlt:  {},
	}
)

type ModifierState struct {
	pressed bool
}

func (m *ModifierState) Press() {
	m.pressed = true
}

func (m *ModifierState) Release() {
	m.pressed = false
}

func (m *ModifierState) IsPressed() bool {
	return m.pressed
}

func (m *ModifierState) IsReleased() bool {
	return !m.pressed
}

type Modifier struct {
	modifiers map[golibevdev.KeyEventCode]*ModifierState
}

func NewModifier() *Modifier {
	modifiers := map[golibevdev.KeyEventCode]*ModifierState{}
	for key := range keys.Modifiers {
		modifiers[key] = &ModifierState{}
	}
	return &Modifier{
		modifiers: modifiers,
	}
}

func (m *Modifier) Press(code golibevdev.KeyEventCode) {
	if !m.IsModifier(code) {
		return
	}
	m.modifiers[code].Press()
}

func (m *Modifier) Release(code golibevdev.KeyEventCode) {
	if !m.IsModifier(code) {
		return
	}
	m.modifiers[code].Release()
}

func (m *Modifier) IsPressed(code golibevdev.KeyEventCode) bool {
	if !m.IsModifier(code) {
		return false
	}
	return m.modifiers[code].IsPressed()
}

func (m *Modifier) IsReleased(code golibevdev.KeyEventCode) bool {
	if !m.IsModifier(code) {
		return false
	}
	return m.modifiers[code].IsReleased()
}

func (m *Modifier) IsModifier(code golibevdev.KeyEventCode) bool {
	_, ok := m.modifiers[code]
	return ok
}

func (m *Modifier) IsAnyModifierPressed() bool {
	for _, state := range m.modifiers {
		if state.IsPressed() {
			return true
		}
	}
	return false
}

func (m *Modifier) ShouldPassThrough(code golibevdev.KeyEventCode) bool {
	_, ok := passThroughKeys[code]
	return ok
}
