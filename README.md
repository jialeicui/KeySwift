# KeySwift

KeySwift is a keyboard remapping tool designed for Linux gnome desktop environments. It allows you to customize keyboard mappings for different applications, enhancing your productivity and typing experience.

## Features

- **Application-specific remapping**: Define custom keyboard mappings for specific applications
- **Flexible configuration**: Simple and intuitive configuration format

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

Create a configuration file at `~/.config/keyswift/config.js`. Here's an example:

```js
const curWindowClass = KeySwift.getActiveWindowClass();
const Terminals = ["kitty", "Gnome-terminal", "org.gnome.Terminal"];
const inTerminal = Terminals.includes(curWindowClass);

KeySwift.onKeyPress(["cmd", "v"], () => {
    if (curWindowClass === "com.mitchellh.ghostty") {
        KeySwift.sendKeys(["shift", "ctrl", "v"]);
        return
    }

    if (inTerminal) {
        KeySwift.sendKeys(["cmd", "shift", "v"]);
    }
});
```

You can also see more examples in the [examples](examples) directory.

KeySwift's config is implemented based on [QuickJS](https://bellard.org/quickjs), and all available objects and functions are as follows:

```js
const KeySwift = {
    getActiveWindowClass: () => string,
    sendKeys: (keys: string[]) => void,
    onKeyPress: (keys: string[], callback: () => void) => void,
}
```

## Acknowledgments

KeySwift was inspired by several excellent projects:

- [xremap](https://github.com/xremap/xremap): A key remapper for X11 and Wayland
- [kmonad](https://github.com/kmonad/kmonad): An advanced keyboard manager with powerful customization features
- [autokey](https://github.com/autokey/autokey): A desktop automation utility for Linux

Thank you to the maintainers of these projects for your contributions to open-source keyboard customization tools!

This project also draws inspiration from [AutoHotkey](https://www.autohotkey.com)'s design philosophy. Thanks to this amazing project

## Tips

### How to get the active window class

You can use the `cmd+i` shortcut to print the active window class to the console with the following configuration:

```js
KeySwift.onKeyPress(["cmd", "i"], () => {
    const curWindowClass = KeySwift.getActiveWindowClass();
    console.log(curWindowClass);
});
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
./keyswift -keyboards XXX -config ~/.config/keyswift/config.js
```
- if you have multiple keyboards, you can use comma to separate them
- if you don't know the device name, you can leave it blank and the program will print all the keyboard device names and you can select one of them

**NOTE**: KeySwift does not support running with sudo, so you need to run the program with current user.

## TODO

- [ ] Support KDE
- [ ] Support Mouse

## Getting Help

If you encounter any issues or have questions, please [open an issue](https://github.com/jialeicui/keyswift/issues) on the GitHub repository.
