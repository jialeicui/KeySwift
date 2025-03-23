package bus

import (
	"sync"

	"github.com/jialeicui/keyswift/pkg/engine"
	"github.com/jialeicui/keyswift/pkg/keys"
)

var _ engine.Bus = (*session)(nil)

type session struct {
	impl        *Impl
	handled     bool
	pressedKeys []keys.Key
	once        sync.Once
	beforeSend  func()
}

func (s *session) GetPressedKeys() []keys.Key {
	return s.pressedKeys
}

func (s *session) GetActiveWindowClass() string {
	return s.impl.GetActiveWindowClass()
}

func (s *session) SendKeys(codes []keys.Key) {
	s.once.Do(s.beforeSend)
	s.handled = true
	s.impl.SendKeys(codes)
}

func (s *session) Handled() bool {
	return s.handled
}

func newSession(m *Impl, pressedKeys []keys.Key, beforeSend func()) *session {
	return &session{
		impl:        m,
		pressedKeys: pressedKeys,
		beforeSend:  beforeSend,
	}
}
