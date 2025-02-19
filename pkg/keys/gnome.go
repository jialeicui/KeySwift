package keys

import (
	"github.com/jialeicui/golibevdev"
)

var _ FunctionKeys = &GnomeKeys{}

type GnomeKeys struct {
	out *golibevdev.UInputDev
}

func NewGnomeKeys(out *golibevdev.UInputDev) *GnomeKeys {
	return &GnomeKeys{
		out: out,
	}
}

// pressWithCtrl simulates pressing a key while holding Ctrl
func (g *GnomeKeys) pressWithCtrl(key golibevdev.KeyEventCode) {
	// Press Ctrl key
	g.out.WriteEvent(golibevdev.EvKey, golibevdev.KeyLeftCtrl, 1)
	// Press target key
	g.out.WriteEvent(golibevdev.EvKey, key, 1)
	// Release target key
	g.out.WriteEvent(golibevdev.EvKey, key, 0)
	// Release Ctrl key
	g.out.WriteEvent(golibevdev.EvKey, golibevdev.KeyLeftCtrl, 0)
}

// pressWithCtrlShift simulates pressing a key while holding Ctrl+Shift
func (g *GnomeKeys) pressWithCtrlShift(key golibevdev.KeyEventCode) {
	// Press Ctrl key
	g.out.WriteEvent(golibevdev.EvKey, golibevdev.KeyLeftCtrl, 1)
	// Press Shift key
	g.out.WriteEvent(golibevdev.EvKey, golibevdev.KeyLeftShift, 1)
	// Press target key
	g.out.WriteEvent(golibevdev.EvKey, key, 1)
	// Release target key
	g.out.WriteEvent(golibevdev.EvKey, key, 0)
	// Release Shift key
	g.out.WriteEvent(golibevdev.EvKey, golibevdev.KeyLeftShift, 0)
	// Release Ctrl key
	g.out.WriteEvent(golibevdev.EvKey, golibevdev.KeyLeftCtrl, 0)
}

func (g *GnomeKeys) Copy() {
	g.pressWithCtrl(golibevdev.KeyC)
}

func (g *GnomeKeys) Cut() {
	g.pressWithCtrl(golibevdev.KeyX)
}

func (g *GnomeKeys) Paste() {
	g.pressWithCtrl(golibevdev.KeyV)
}

func (g *GnomeKeys) Undo() {
	g.pressWithCtrl(golibevdev.KeyZ)
}

func (g *GnomeKeys) Redo() {
	g.pressWithCtrlShift(golibevdev.KeyZ)
}

func (g *GnomeKeys) Find() {
	g.pressWithCtrl(golibevdev.KeyF)
}

func (g *GnomeKeys) Replace() {
	g.pressWithCtrl(golibevdev.KeyH)
}

func (g *GnomeKeys) SelectAll() {
	g.pressWithCtrl(golibevdev.KeyA)
}

func (g *GnomeKeys) Open() {
	g.pressWithCtrl(golibevdev.KeyO)
}

func (g *GnomeKeys) Close() {
	g.pressWithCtrl(golibevdev.KeyW)
}

func (g *GnomeKeys) Save() {
	g.pressWithCtrl(golibevdev.KeyS)
}

func (g *GnomeKeys) SaveAs() {
	g.pressWithCtrlShift(golibevdev.KeyS)
}

func (g *GnomeKeys) Print() {
	g.pressWithCtrl(golibevdev.KeyP)
}

func (g *GnomeKeys) Quit() {
	g.pressWithCtrl(golibevdev.KeyQ)
}
