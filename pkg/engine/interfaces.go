package engine

import (
	"github.com/jialeicui/keyswift/pkg/keys"
)

const (
	FuncGetActiveWindowClass = "getActiveWindowClass"
	FuncSendKeys             = "sendKeys"
	FuncOnKeyPress           = "onKeyPress"

	KeySwiftObj = "KeySwift"
)

type Engine interface {
	Run(session Bus) error
	Release()
}

type Bus interface {
	GetActiveWindowClass() string
	GetPressedKeys() []keys.Key
	SendKeys(keys []keys.Key)
}
