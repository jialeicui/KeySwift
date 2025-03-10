# KeySwift

KeySwift is a keyboard remapping tool designed for Linux gnome desktop environments. It allows you to customize keyboard mappings for different applications, enhancing your productivity and typing experience.

## Features

- **Application-specific remapping**: Define custom keyboard mappings for specific applications
- **Flexible configuration**: Simple and intuitive configuration format

## Installation

Download the latest release from the [Releases](https://github.com/jialeicui/keyswift/releases) page.

Or install from source:

```bash
git clone https://github.com/jialeicui/keyswift.git
cd keyswift
make install
```

## Configuration

Create a configuration file at `~/.config/keyswift/config.yml`. Here's an example:

```yaml
# yaml-language-server: $schema=schema.json
# KeySwift Configuration File

mode_actions:
  default:
    if: "true"
    triggers:
      - source_event:
          key_press_event:
            key: "esc"
        action:
          set_value:
            normal: true
      - source_event:
          window_focus_event:
            window_class:
              - kitty
        action:
          set_value:
            terminal: true
  normal:
    if: normal
    triggers:
      - source_event:
          key_press_event:
            key: "j"
        action:
          map_to_keys:
            keys:
              - key: "down"
```

## Acknowledgments

KeySwift was inspired by several excellent projects:

- [xremap](https://github.com/xremap/xremap): A key remapper for X11 and Wayland
- [kmonad](https://github.com/kmonad/kmonad): An advanced keyboard manager with powerful customization features
- [autokey](https://github.com/autokey/autokey): A desktop automation utility for Linux

Thank you to the maintainers of these projects for your contributions to open-source keyboard customization tools!

## Getting Help

If you encounter any issues or have questions, please [open an issue](https://github.com/jialeicui/keyswift/issues) on the GitHub repository.
