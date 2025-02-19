package keys

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
}
