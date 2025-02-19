package dbus

import (
	"encoding/json"
	"fmt"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"

	"github.com/jialeicui/keyswift/pkg/wininfo"
)

var _ wininfo.WinGetter = (*Receiver)(nil)

const (
	BusName       = "com.github.keyswift.WindowMonitor"
	BusPath       = "/com/github/keyswift/WindowMonitor"
	BusInterface  = "com.github.keyswift.WindowMonitor"
	introspectXML = `
<node>
    <interface name="com.github.keyswift.WindowMonitor">
        <method name="UpdateActiveWindow">
            <arg type="s" direction="in"/>
        </method>
    </interface>` + introspect.IntrospectDataString + `
</node>`
)

type Receiver struct {
	*options
	current *wininfo.WinInfo
	conn    *dbus.Conn
}

func (r *Receiver) UpdateActiveWindow(in string) *dbus.Error {
	info := new(Info)
	if err := json.Unmarshal([]byte(in), info); err != nil {
		return dbus.NewError("com.github.keyswift.WindowMonitor.Error", []interface{}{err.Error()})
	}

	r.current = &wininfo.WinInfo{
		Title: info.Title,
		Class: info.Class,
	}

	if r.options.onChange != nil {
		r.options.onChange(r.current)
	}

	return nil
}

func (r *Receiver) GetActiveWindow() (*wininfo.WinInfo, error) {
	if r.current == nil {
		return nil, fmt.Errorf("no active window")
	}
	return r.current, nil
}

func (r *Receiver) Close() {
	r.conn.Close()
}

func (r *Receiver) setupDBus() error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}

	reply, err := conn.RequestName(BusName, dbus.NameFlagDoNotQueue)
	if err != nil {
		return err
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		return fmt.Errorf("name already taken")
	}

	if err = conn.Export(r, BusPath, BusInterface); err != nil {
		return err
	}

	if err = conn.Export(introspect.Introspectable(introspectXML), BusPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return err
	}

	r.conn = conn
	return nil
}

func (r *Receiver) OnActiveWindowChange(callback wininfo.ActiveWindowChangeCallback) error {
	r.options.onChange = callback
	return nil
}

type Info struct {
	Title string `json:"title"`
	Class string `json:"class"`
}

type options struct {
	onChange wininfo.ActiveWindowChangeCallback
}

type Option func(*options)

func WithActiveWindowChangeCallback(callback wininfo.ActiveWindowChangeCallback) Option {
	return func(o *options) {
		o.onChange = callback
	}
}

func New(opt... Option) (*Receiver, error) {
	r := &Receiver{
		options: &options{},
	}


	for _, o := range opt {
		o(r.options)
	}

	err := r.setupDBus()
	if err != nil {
		return nil, err
	}

	return r, nil
}
