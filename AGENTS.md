# KeySwift - Agent Guide

## Project Overview

KeySwift is a Linux keyboard remapping tool designed specifically for GNOME desktop environments. It enables application-specific key mappings using a JSON configuration file, allowing users to customize keyboard shortcuts for different applications.

### Key Features
- **Application-specific remapping**: Define custom keyboard mappings for specific applications based on window class
- **JSON configuration**: Simple, static configuration using a JSON array of rules
- **Low-level input handling**: Uses Linux evdev subsystem for direct keyboard input capture
- **D-Bus integration**: Communicates with GNOME Shell extension for window focus tracking

## Technology Stack

- **Language**: Go 1.22+
- **System Dependencies**: 
  - libevdev-dev (Linux input event handling)
  - D-Bus (GNOME integration)
- **Key Go Dependencies**:
  - `github.com/godbus/dbus/v5` - D-Bus bindings for Go
  - `github.com/jialeicui/golibevdev` - Linux evdev wrapper
  - `github.com/stretchr/testify` - Testing framework

## Project Structure

```
.
├── cmd/keyswift/main.go          # Application entry point and CLI
├── pkg/
│   ├── config/                   # Configuration loading and parsing
│   │   └── types.go              # Config types and JSON loader
│   ├── statemachine/             # Key event processing
│   │   ├── machine.go            # Core state machine
│   │   ├── config_engine.go      # Config-based mapping engine
│   │   ├── config_integration.go # State machine + config integration
│   │   ├── handler_integration.go# Input device management
│   │   ├── adapter.go            # Output device adapter
│   │   └── interfaces.go         # Core interfaces
│   ├── evdev/                    # Input device management
│   │   ├── evdev.go              # Core types
│   │   └── overview.go           # Device enumeration
│   ├── keys/                     # Key code mappings
│   │   └── keys.go               # Key name to code conversion
│   ├── utils/                    # Utilities
│   │   └── config.go             # Configuration path helpers
│   └── wininfo/                  # Window information
│       ├── wininfo.go            # Interface definitions
│       └── dbus/                 # D-Bus implementation
├── examples/
│   ├── config.json               # Example configuration (compact)
│   └── config.json5              # Example configuration (annotated)
├── Makefile                      # Build automation
└── go.mod                        # Go module definition
```

## Architecture

### Data Flow

1. **Input Capture** (`pkg/statemachine/handler_integration.go`)
   - Grabs physical keyboard devices via evdev
   - Processes raw key events
   - Manages device reconnection

2. **Event Processing** (`pkg/statemachine/`)
   - State machine receives key events
   - Evaluates mapping rules against current key state and window class
   - Routes output commands to virtual keyboard

3. **Config Mapping Engine** (`pkg/statemachine/config_engine.go`)
   - Loads static rules from `pkg/config/`
   - Set-based matching: key combinations are unordered sets
   - Filters by window class conditions (`window` / `notWindow`)

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

### JSON Format

The configuration file (`~/.config/keyswift/config.json`) is a JSON array of rule objects.

**Rule types:**

```json
[
  {"type": "var", "name": "terminals", "value": ["kitty", "Gnome-terminal"]},
  {
    "type": "map",
    "input": ["cmd", "c"],
    "output": ["ctrl", "c"],
    "when": {"notWindow": "$terminals"}
  }
]
```

- **`var`**: Defines a named list of window classes (`$varname` reference)
- **`map`**: Maps an input key combo to an output combo, with optional `when` condition
  - `when.window`: apply only when active window class matches
  - `when.notWindow`: apply only when active window class does **not** match

### Example Configuration

See `examples/config.json` and `examples/config.json5` (annotated) for comprehensive examples including:
- Terminal-specific mappings (kitty, GNOME Terminal, Ghostty)
- IDE mappings (JetBrains)
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
./keyswift -keyboards "HHKB" -config ~/.config/keyswift/config.json

# Multiple keyboards (comma-separated)
./keyswift -keyboards "HHKB,Logitech" -config ~/.config/keyswift/config.json

# Verbose logging
./keyswift -keyboards "HHKB" -config ~/.config/keyswift/config.json -verbose

# Custom output device name
./keyswift -keyboards "HHKB" -output-device-name "my-keyboard" -config ~/.config/keyswift/config.json
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
- Interfaces with `-er` suffix (e.g., `WinGetter`, `MappingEngine`)
- Implementation types with descriptive names (e.g., `Impl`, `ConfigMappingEngine`, `Receiver`)
- Constants for key codes and configuration field names

### Error Handling
- Return errors with context
- Use `slog.Error()` for operational errors
- Graceful degradation (e.g., `DegradedReceiver` for D-Bus failures)

## Security Considerations

1. **Input Device Access**: Requires membership in `input` group
2. **D-Bus Communication**: Exposes service on session bus
3. **Configuration**: JSON config is parsed at startup; no code execution at runtime

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
