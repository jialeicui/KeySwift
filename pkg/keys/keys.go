package keys

import "github.com/jialeicui/golibevdev"

type FunctionKeys interface {
	Copy()
	Cut()
	Paste()
	Undo()
	Redo()
	Find()
	Replace()
	SelectAll()
	Open()
	Close()
	Save()
	SaveAs()
	Print()
	Quit()
	SendKeys(keys []golibevdev.KeyEventCode) error
}
