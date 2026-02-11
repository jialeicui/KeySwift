# KeySwift

KeySwift is a keyboard remapping tool designed for Linux GNOME desktop environments. It allows you to customize keyboard mappings for different applications, enhancing your productivity and typing experience.

## Features

- **Application-specific remapping**: Define custom keyboard mappings for specific applications
- **Flexible configuration**: Simple and intuitive JSON configuration format

## Installation

Download the latest release from the [Releases](https://github.com/jialeicui/keyswift/releases) page.

Or build from source:

**Dependencies**:
- libevdev-dev
- golang


```bash
git clone https://github.com/jialeicui/keyswift.git
cd keyswift
make
```

## Configuration

Create a configuration file at `~/.config/keyswift/config.json`. The configuration is a JSON array of rule objects.

Each rule has a `type` field:
- `"var"` — define a named variable (list of window classes) for reuse
- `"map"` — define a key mapping with optional window conditions

### Example

```json
[
  {"type": "var", "name": "terminals", "value": ["kitty", "Gnome-terminal", "org.gnome.Terminal"]},

  {"type": "map", "input": ["cmd", "c"], "output": ["ctrl", "c"],       "when": {"notWindow": "$terminals"}},
  {"type": "map", "input": ["cmd", "c"], "output": ["ctrl", "shift", "c"], "when": {"window": "$terminals"}},
  {"type": "map", "input": ["cmd", "v"], "output": ["ctrl", "v"],       "when": {"notWindow": "$terminals"}},
  {"type": "map", "input": ["cmd", "v"], "output": ["ctrl", "shift", "v"], "when": {"window": "$terminals"}}
]
```

See the [examples](examples) directory for a more comprehensive configuration.

### Key Names (case-insensitive)

| Category   | Names |
|------------|-------|
| Modifiers  | `ctrl`, `alt`, `cmd`/`meta`/`super`, `shift` |
| Letters    | `a`–`z` |
| Numbers    | `0`–`9` |
| Function   | `f1`–`f12` |
| Navigation | `home`, `end`, `pageup`, `pagedown`, `up`, `down`, `left`, `right` |
| Special    | `esc`, `tab`, `space`, `enter`, `backspace`, `delete`, `insert` |

### Conditions (`when`)

- `window`: mapping applies only when the active window class matches (string or array)
- `notWindow`: mapping applies when the active window class does **not** match (string or array)
- Use `$varname` to reference a previously-defined `var` rule

## Acknowledgments

KeySwift was inspired by several excellent projects:

- [xremap](https://github.com/xremap/xremap): A key remapper for X11 and Wayland
- [kmonad](https://github.com/kmonad/kmonad): An advanced keyboard manager with powerful customization features
- [autokey](https://github.com/autokey/autokey): A desktop automation utility for Linux

Thank you to the maintainers of these projects for your contributions to open-source keyboard customization tools!

This project also draws inspiration from [AutoHotkey](https://www.autohotkey.com)'s design philosophy. Thanks to this amazing project.

## Tips

### How to get the active window class

Run the following command to print all visible window classes so you can find the right name for your application:

```bash
xprop WM_CLASS | awk '{print $NF}'
# Then click the window you want to identify
```

### How to run the program

1. Install the keyswift gnome extension

```bash
# Clone the repository
git clone https://github.com/jialeicui/keyswift-gnome-ext.git ~/.local/share/gnome-shell/extensions/keyswift@jialeicui.github.io
# Enable the extension via gnome-extensions-app or gnome-extensions cli
gnome-extensions enable keyswift@jialeicui.github.io
# You may need to restart the gnome-shell
```

2. Get the input device permission for current user

```bash
sudo gpasswd -a $USER input
echo 'KERNEL=="uinput", GROUP="input", TAG+="uaccess"' | sudo tee /etc/udev/rules.d/input.rules
# You may need to restart the system to take effect
```

3. Run the program

```bash
# XXX is the substring of the keyboard device name
./keyswift -keyboards XXX -config ~/.config/keyswift/config.json
```
- if you have multiple keyboards, you can use comma to separate them
- if you don't know the device name, you can leave it blank and the program will print all the keyboard device names and you can select one of them

**NOTE**: KeySwift does not support running with sudo, so you need to run the program with current user.

## TODO

- [ ] Support KDE
- [ ] Support Mouse

## Getting Help

If you encounter any issues or have questions, please [open an issue](https://github.com/jialeicui/keyswift/issues) on the GitHub repository.
