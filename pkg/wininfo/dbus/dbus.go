package dbus

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"

	"github.com/jialeicui/keyswift/pkg/wininfo"
)

var _ wininfo.WinGetter = (*Receiver)(nil)

const (
	BusName       = "com.github.keyswift.WinInfoReceiver"
	BusPath       = "/com/github/keyswift/WinInfoReceiver"
	BusInterface  = "com.github.keyswift.WinInfoReceiver"
	introspectXML = `
<node>
    <interface name="com.github.keyswift.WinInfoReceiver">
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
		return dbus.NewError("com.github.keyswift.WinInfoReceiver.Error", []any{err.Error()})
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

// getDBusAddress attempts to get the latest DBus address
func getDBusAddress() (string, error) {
	// 1. First try to get from environment variable
	if addr := os.Getenv("DBUS_SESSION_BUS_ADDRESS"); addr != "" {
		return addr, nil
	}

	// 2. Try to get from systemd user session
	uid := os.Getuid()
	systemdSocket := fmt.Sprintf("/run/user/%d/bus", uid)
	if _, err := os.Stat(systemdSocket); err == nil {
		return fmt.Sprintf("unix:path=%s", systemdSocket), nil
	}

	// 3. Try to get from session file
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	// Get DISPLAY environment variable, use ":0" as default if not set
	display := os.Getenv("DISPLAY")
	if display == "" {
		display = ":0"
	}

	// Try to get from session file
	sessionFile := filepath.Join(home, ".dbus", "session-bus", display)
	if _, err := os.Stat(sessionFile); err == nil {
		content, err := os.ReadFile(sessionFile)
		if err != nil {
			return "", fmt.Errorf("failed to read session file: %w", err)
		}

		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "DBUS_SESSION_BUS_ADDRESS=") {
				addr := strings.TrimPrefix(line, "DBUS_SESSION_BUS_ADDRESS=")
				addr = strings.Trim(addr, "'\"")
				return addr, nil
			}
		}
	}

	// 4. Try to get from XDG_RUNTIME_DIR
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		busPath := filepath.Join(runtimeDir, "bus")
		if _, err := os.Stat(busPath); err == nil {
			return fmt.Sprintf("unix:path=%s", busPath), nil
		}
	}

	return "", fmt.Errorf("failed to find DBus address")
}

func (r *Receiver) setupDBus() error {
	// Get the latest DBus address
	addr, err := getDBusAddress()
	if err != nil {
		return fmt.Errorf("failed to get DBus address: %w", err)
	}

	// Set environment variable
	os.Setenv("DBUS_SESSION_BUS_ADDRESS", addr)

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

// DegradedReceiver is a degraded mode implementation, used when DBus is unavailable
type DegradedReceiver struct {
	*options
	current *wininfo.WinInfo
}

func (r *DegradedReceiver) UpdateActiveWindow(in string) *dbus.Error {
	return nil
}

func (r *DegradedReceiver) GetActiveWindow() (*wininfo.WinInfo, error) {
	if r.current == nil {
		return nil, fmt.Errorf("no active window")
	}
	return r.current, nil
}

func (r *DegradedReceiver) Close() {
}

func (r *DegradedReceiver) OnActiveWindowChange(callback wininfo.ActiveWindowChangeCallback) error {
	r.options.onChange = callback
	return nil
}

func New(opt ...Option) (wininfo.WinGetter, error) {
	r := &Receiver{
		options: &options{},
	}

	for _, o := range opt {
		o(r.options)
	}

	err := r.setupDBus()
	if err != nil {
		// If DBus connection fails, return degraded mode implementation
		slog.Warn("Failed to connect to DBus, running in degraded mode", "error", err)
		return &DegradedReceiver{
			options: r.options,
		}, nil
	}

	return r, nil
}
