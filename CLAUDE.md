# KeySwift Project Overview

## What is KeySwift
A Linux keyboard remapping tool for GNOME environments that allows application-specific key mappings using JavaScript configuration.

## Quick Start
```bash
# Build
make

# Run (after setup)
./keyswift -keyboards "HHKB" -config ~/.config/keyswift/config.js
```

## Architecture
- **Language**: Go 1.22 + QuickJS (JavaScript engine)
- **Core Components**:
  - `cmd/keyswift/main.go` - Entry point and CLI
  - `pkg/bus/` - Event processing and JS engine integration
  - `pkg/handler/` - Input device management
  - `pkg/evdev/` - Linux input device handling
  - `pkg/engine/` - QuickJS JavaScript engine
  - `pkg/wininfo/` - Window context detection via D-Bus

## Key Files
- `examples/config.js` - Configuration examples
- `README.md` - Complete setup guide
- `Makefile` - Build configuration
- `pkg/handler/keystate.go` - New key state management system

## Key Handling Improvements (July 2025)
- **KeyStateManager**: Added proper physical/virtual keyboard state synchronization
- **Debouncing**: 5ms threshold to prevent rapid key event issues
- **State Sync**: 100ms periodic synchronization to prevent stuck keys
- **Emergency Reset**: Automatic key release on shutdown
- **Event Ordering**: Improved modifier handling with proper press/release sequences
- **Duplicate Prevention**: Eliminates duplicate key events in remapped combinations

## Key APIs (JavaScript)
```js
KeySwift.getActiveWindowClass() // Get current app
KeySwift.sendKeys(["ctrl", "c"]) // Send key combo
KeySwift.onKeyPress(["cmd", "v"], callback) // Bind keys
```

## Setup Requirements
1. Install GNOME extension: `keyswift-gnome-ext`
2. Add user to `input` group
3. Configure udev rules for input access
4. Create `~/.config/keyswift/config.js`

## Dependencies
- libevdev-dev
- golang 1.22+
- godbus/dbus/v5
- QuickJS-go