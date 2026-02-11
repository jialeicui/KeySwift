# KeySwift - Agent Guide

## Project Overview

KeySwift is a Linux keyboard remapping tool designed specifically for GNOME desktop environments. It enables application-specific key mappings using JavaScript configuration files, allowing users to customize keyboard shortcuts for different applications.

### Key Features
- **Application-specific remapping**: Define custom keyboard mappings for specific applications based on window class
- **JavaScript configuration**: Flexible configuration using QuickJS JavaScript engine
- **Low-level input handling**: Uses Linux evdev subsystem for direct keyboard input capture
- **D-Bus integration**: Communicates with GNOME Shell extension for window focus tracking

## Technology Stack

- **Language**: Go 1.22+
- **JavaScript Engine**: QuickJS (via buke/quickjs-go)
- **System Dependencies**: 
  - libevdev-dev (Linux input event handling)
  - D-Bus (GNOME integration)
- **Key Go Dependencies**:
  - `github.com/godbus/dbus/v5` - D-Bus bindings for Go
  - `github.com/buke/quickjs-go` - QuickJS JavaScript engine bindings
  - `github.com/jialeicui/golibevdev` - Linux evdev wrapper
  - `github.com/samber/lo` - Go utilities library
  - `github.com/stretchr/testify` - Testing framework

## Project Structure

```
.
├── cmd/keyswift/main.go          # Application entry point and CLI
├── pkg/
│   ├── bus/                      # Event processing and coordination
│   │   ├── impl.go               # Main bus implementation
│   │   ├── mode.go               # Event type definitions
│   │   └── session.go            # Per-event processing session
│   ├── engine/                   # JavaScript engine integration
│   │   ├── interfaces.go         # Engine and Bus interfaces
│   │   └── quickjs.go            # QuickJS implementation
│   ├── evdev/                    # Input device management
│   │   ├── evdev.go              # Core types
│   │   └── overview.go           # Device enumeration
│   ├── handler/                  # Input event handling
│   │   ├── handler.go            # Main event processor
│   │   └── modifier.go           # Modifier key state tracking
│   ├── keys/                     # Key code mappings
│   │   └── keys.go               # Key name to code conversion
│   ├── utils/                    # Utilities
│   │   ├── config.go             # Configuration path helpers
│   │   └── cache/                # Caching utilities
│   └── wininfo/                  # Window information
│       ├── wininfo.go            # Interface definitions
│       └── dbus/                 # D-Bus implementation
├── examples/
│   └── config.js                 # Example configuration
├── Makefile                      # Build automation
└── go.mod                        # Go module definition
```

## Architecture

### Data Flow

1. **Input Capture** (`pkg/handler/`)
   - Grabs physical keyboard devices via evdev
   - Processes raw key events
   - Tracks modifier key states
   - Manages key press/release sequences

2. **Event Processing** (`pkg/bus/`)
   - Receives key events from handler
   - Creates isolated session per event
   - Executes JavaScript configuration
   - Routes output to virtual keyboard

3. **JavaScript Engine** (`pkg/engine/`)
   - Compiles user configuration to bytecode
   - Exposes `KeySwift` global object with APIs:
     - `getActiveWindowClass()` - Get current application
     - `sendKeys(keys[])` - Send key combination
     - `onKeyPress(keys[], callback)` - Register key handler
   - Fast-path filtering for unregistered key combinations

4. **Window Detection** (`pkg/wininfo/`)
   - D-Bus service receives window info from GNOME extension
   - Falls back to degraded mode if D-Bus unavailable
   - Provides active window class for conditional mappings

5. **Output** (via golibevdev)
   - Creates virtual keyboard device
   - Sends remapped key events
   - Proper modifier handling (press/release ordering)

## Build Commands

```bash
# Build the binary
make

# Build with version info
go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)" -o keyswift cmd/keyswift/main.go

# Run tests (requires sudo for evdev access)
sudo go test -v ./...

# Run specific package tests
sudo go test -v ./pkg/utils/cache/
```

## Testing

### Test Structure
- Unit tests use `github.com/stretchr/testify`
- Tests requiring evdev access need `sudo`
- Key test files:
  - `pkg/utils/cache/cache_test.go` - Cache implementation tests
  - `pkg/evdev/overview_test.go` - Device enumeration tests

### CI/CD
- GitHub Actions workflow in `.github/workflows/unit-test.yml`
- Runs on push to main and pull requests
- Requires `libevdev-dev` package
- Tests run with Go 1.23

## Configuration

### JavaScript API

The configuration file (`~/.config/keyswift/config.js`) has access to:

```javascript
const KeySwift = {
    // Returns the window class of the currently focused application
    getActiveWindowClass: () => string,
    
    // Sends a key combination (modifiers: ctrl, alt, cmd/meta/super, shift)
    sendKeys: (keys: string[]) => void,
    
    // Registers a callback for specific key combination
    // Must be called at top level, not inside callbacks or conditionals
    onKeyPress: (keys: string[], callback: () => void) => void,
}
```

### Example Configuration

See `examples/config.js` for a comprehensive example including:
- Terminal-specific mappings (kitty, GNOME Terminal, Ghostty)
- IDE mappings (JetBrains, Cursor, Sublime Text)
- macOS-like shortcuts for Linux
- Chrome tab switching shortcuts
- Emacs-style navigation

## Runtime Requirements

### System Setup
1. **GNOME Extension**: Install `keyswift-gnome-ext` from https://github.com/jialeicui/keyswift-gnome-ext
2. **User Permissions**: Add user to `input` group
3. **udev Rules**: Create `/etc/udev/rules.d/input.rules`:
   ```
   KERNEL=="uinput", GROUP="input", TAG+="uaccess"
   ```
4. **Restart** system to apply group and udev changes

### Running

```bash
# List available keyboards and filter by pattern
./keyswift -keyboards "HHKB" -config ~/.config/keyswift/config.js

# Multiple keyboards (comma-separated)
./keyswift -keyboards "HHKB,Logitech" -config ~/.config/keyswift/config.js

# Verbose logging
./keyswift -keyboards "HHKB" -config ~/.config/keyswift/config.js -verbose

# Custom output device name
./keyswift -keyboards "HHKB" -output-device-name "my-keyboard" -config ~/.config/keyswift/config.js
```

**Important**: Do not run with `sudo`. The application requires user-level permissions with `input` group membership.

## Code Style Guidelines

### Go Conventions
- Standard Go formatting (`gofmt`)
- Package comments for all public packages
- Interface definitions in dedicated files (e.g., `interfaces.go`)
- Error wrapping with context using `fmt.Errorf("...: %w", err)`
- Structured logging using `log/slog`

### Naming
- Interfaces with `-er` suffix (e.g., `WinGetter`, `Engine`)
- Implementation types with descriptive names (e.g., `Impl`, `QuickJS`, `Receiver`)
- Constants for magic strings (e.g., `FuncSendKeys`, `KeySwiftObj`)

### Error Handling
- Return errors with context
- Use `slog.Error()` for operational errors
- Graceful degradation (e.g., `DegradedReceiver` for D-Bus failures)

## Security Considerations

1. **Input Device Access**: Requires membership in `input` group
2. **D-Bus Communication**: Exposes service on session bus
3. **JavaScript Execution**: User-provided scripts run in QuickJS sandbox with memory limits:
   - Memory limit: 1280 KB
   - GC threshold: 2560 KB
   - Max stack size: 65534
   - Execution timeout: 0 (disabled)

## Key Implementation Details

### Modifier Key Handling
- Modifier keys (Ctrl, Alt, Meta) are tracked separately
- Pass-through mode for modifiers enables browser shortcuts (e.g., Ctrl+Click)
- Modifier release events are synthesized when remapping begins

### Key State Management
- Debouncing with 5ms threshold
- Periodic state synchronization (100ms)
- Emergency key release on shutdown
- Duplicate event prevention

### Fast Path Optimization
- JavaScript engine tracks registered key combinations
- Unregistered combinations bypass script execution
- Key code caching to avoid repeated lookups

## Related Projects

- **GNOME Extension**: https://github.com/jialeicui/keyswift-gnome-ext
- Inspired by: xremap, kmonad, autokey, AutoHotkey
