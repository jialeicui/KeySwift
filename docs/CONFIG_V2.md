# KeySwift Configuration Format V2

## Overview

KeySwift now uses a **declarative JSON-based configuration** that is loaded once at startup and compiled into static rules for the State Machine. This provides:

- **Better performance**: No JavaScript runtime overhead during key processing
- **No sticky keys**: State Machine architecture with proper modifier handling
- **Type safety**: Configuration is validated at load time
- **Predictable behavior**: All rules are known at startup

## Quick Start

1. Create `~/.config/keyswift/config.json`:
```json
[
  {"type": "var", "name": "terminals", "value": ["kitty", "Gnome-terminal"]},
  {"type": "map", "input": ["cmd", "c"], "output": ["ctrl", "c"]},
  {"type": "map", "input": ["cmd", "v"], "output": ["ctrl", "v"]}
]
```

2. Run KeySwift with state machine enabled (default):
```bash
./keyswift -keyboards "HHKB" -config ~/.config/keyswift/config.json
```

## Configuration Format

Configuration is a JSON array of rule objects. Each rule has a `type` field.

### Variable Definition (`type: "var"`)

Define reusable groups of window classes:

```json
{
  "type": "var",
  "name": "terminals",
  "value": ["kitty", "Gnome-terminal", "org.gnome.Terminal"]
}
```

Variables can reference other variables using `$varname`:

```json
{
  "type": "var",
  "name": "vimMode",
  "value": ["Cursor", "$jetbrains"]
}
```

### Key Mapping (`type: "map"`)

Define key remapping rules:

```json
{
  "type": "map",
  "input": ["cmd", "c"],
  "output": ["ctrl", "c"],
  "when": {
    "window": "Google-chrome",
    "notWindow": ["$terminals", "kitty"]
  }
}
```

Fields:
- `input` (required): Array of key names to match
- `output` (required): Array of key names to send
- `when` (optional): Conditions for when this mapping applies

### Conditions (`when`)

- `window`: Only match when window class is in the list
- `notWindow`: Only match when window class is NOT in the list

Both accept a single string or an array of strings. Variable references (`$varname`) are supported.

## Key Names

Key names are case-insensitive. Common names:

### Modifiers
- `ctrl`, `ctrl-l`, `lctrl`, `ctrl-r`, `rctrl` - Control keys
- `alt`, `alt-l`, `lalt`, `alt-r`, `ralt` - Alt keys
- `cmd`, `meta`, `super`, `lcmd`, `lmeta`, `lsuper` - Left Meta/Super/Cmd
- `rcmd`, `rmeta`, `rsuper` - Right Meta/Super/Cmd
- `shift`, `shift-l`, `lshift`, `shift-r`, `rshift` - Shift keys

### Alphanumeric
- Letters: `a` through `z`
- Numbers: `0` through `9`
- Function keys: `f1` through `f12`

### Special Keys
- `esc`, `escape` - Escape
- `tab` - Tab
- `space` - Spacebar
- `enter`, `return` - Enter/Return
- `backspace` - Backspace
- `delete`, `del` - Delete
- `insert`, `ins` - Insert
- `home` - Home
- `end` - End
- `pageup`, `pgup` - Page Up
- `pagedown`, `pgdn` - Page Down
- `up`, `down`, `left`, `right` - Arrow keys
- `capslock` - Caps Lock

### Numpad
- `kp0` through `kp9` - Numpad numbers
- `kpenter` - Numpad Enter
- `kpplus`, `kpminus`, `kpasterisk`, `kpslash` - Numpad operators
- `kpdot` - Numpad Dot

### Media Keys (if supported by your keyboard)
- `mute`, `volumedown`, `volumeup` - Volume control
- `playpause`, `stop`, `previoussong`, `nextsong` - Media control

## Complete Example

See `examples/config.json5` for a comprehensive, commented example configuration.

## Migration from Old JavaScript Config

Old config (JavaScript):
```javascript
const Terminals = ["kitty", "Gnome-terminal"];

KeySwift.onKeyPress(["cmd", "c"], () => {
    if (Terminals.includes(curWindowClass)) {
        KeySwift.sendKeys(["ctrl", "shift", "c"]);
    } else {
        KeySwift.sendKeys(["ctrl", "c"]);
    }
});
```

New config (JSON):
```json
[
  {"type": "var", "name": "terminals", "value": ["kitty", "Gnome-terminal"]},
  {"type": "map", "input": ["cmd", "c"], "output": ["ctrl", "c"], "when": {"notWindow": "$terminals"}},
  {"type": "map", "input": ["cmd", "c"], "output": ["ctrl", "shift", "c"], "when": {"window": "$terminals"}}
]
```

Key differences:
1. No JavaScript code, pure declarative JSON
2. Multiple rules with conditions instead of if-else
3. Order matters: first matching rule wins

## Debugging

Use `-verbose` flag to see detailed matching:

```bash
./keyswift -keyboards "HHKB" -config ~/.config/keyswift/config.json -verbose
```

You'll see logs like:
```
msg="Mapping matched" input=[KeyLeftMeta,KeyC] output=[KeyLeftCtrl,KeyC] window=Google-chrome
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     KeySwift Process                         │
├─────────────────────────────────────────────────────────────┤
│  Load Phase (Once at startup)                                │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │ config.json  │───▶│  Parser      │───▶│  Static      │  │
│  │              │    │              │    │  Rules       │  │
│  └──────────────┘    └──────────────┘    └──────────────┘  │
├─────────────────────────────────────────────────────────────┤
│  Runtime Phase (Per key event)                               │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │ Input Device │───▶│ State Machine│───▶│ Output Device│  │
│  │              │    │ + Rules      │    │              │  │
│  └──────────────┘    └──────────────┘    └──────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

The State Machine handles:
- Modifier state tracking (pending/active/independent)
- Debouncing
- Key combination matching
- Window class filtering
- Output command generation

No JavaScript is executed at runtime, ensuring consistent low latency and no sticky keys.
