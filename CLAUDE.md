# KeySwift Project Overview

## What is KeySwift
A Linux keyboard remapping tool for GNOME environments that allows application-specific key mappings using a JSON configuration file.

## Quick Start
```bash
# Build
make

# Run (after setup)
./keyswift -keyboards "HHKB" -config ~/.config/keyswift/config.json
```

## Architecture
- **Language**: Go 1.22+
- **Core Components**:
  - `cmd/keyswift/main.go` - Entry point and CLI
  - `pkg/config/` - JSON configuration loading and parsing
  - `pkg/statemachine/` - State machine for key event processing
  - `pkg/evdev/` - Linux input device handling
  - `pkg/wininfo/` - Window context detection via D-Bus
  - `pkg/keys/` - Key name to evdev code mapping

## Key Files
- `examples/config.json` - Configuration example
- `examples/config.json5` - Annotated configuration example
- `README.md` - Complete setup guide
- `Makefile` - Build configuration

## Configuration Format (JSON)
The config is a JSON array of rule objects:
```json
[
  {"type": "var", "name": "terminals", "value": ["kitty", "Gnome-terminal"]},
  {"type": "map", "input": ["cmd", "c"], "output": ["ctrl", "c"], "when": {"notWindow": "$terminals"}}
]
```

Rule types:
- `var` - Named list of window classes for reuse (`$varname`)
- `map` - Key mapping with optional `when.window` / `when.notWindow` conditions

## Setup Requirements
1. Install GNOME extension: `keyswift-gnome-ext`
2. Add user to `input` group
3. Configure udev rules for input access
4. Create `~/.config/keyswift/config.json`

## Dependencies
- libevdev-dev
- golang 1.22+
- godbus/dbus/v5
- jialeicui/golibevdev