package keys

import (
	"fmt"
	"strings"

	"github.com/jialeicui/golibevdev"
)

type Key = golibevdev.KeyEventCode

var Modifiers = map[Key]struct{}{
	golibevdev.KeyLeftShift:  {},
	golibevdev.KeyLeftCtrl:   {},
	golibevdev.KeyLeftAlt:    {},
	golibevdev.KeyLeftMeta:   {},
	golibevdev.KeyRightShift: {},
	golibevdev.KeyRightCtrl:  {},
	golibevdev.KeyRightAlt:   {},
}

var keyMap = map[string]Key{
	"ctrl":    golibevdev.KeyLeftCtrl,
	"l-ctrl":  golibevdev.KeyLeftCtrl,
	"lctrl":   golibevdev.KeyLeftCtrl,
	"alt":     golibevdev.KeyLeftAlt,
	"l-alt":   golibevdev.KeyLeftAlt,
	"lalt":    golibevdev.KeyLeftAlt,
	"cmd":     golibevdev.KeyLeftMeta,
	"l-cmd":   golibevdev.KeyLeftMeta,
	"lcmd":    golibevdev.KeyLeftMeta,
	"meta":    golibevdev.KeyLeftMeta,
	"l-meta":  golibevdev.KeyLeftMeta,
	"lmeta":   golibevdev.KeyLeftMeta,
	"super":   golibevdev.KeyLeftMeta,
	"l-super": golibevdev.KeyLeftMeta,
	"lsuper":  golibevdev.KeyLeftMeta,
	"shift":   golibevdev.KeyLeftShift,
	"l-shift": golibevdev.KeyLeftShift,
	"lshift":  golibevdev.KeyLeftShift,
	"r-ctrl":  golibevdev.KeyRightCtrl,
	"rctrl":   golibevdev.KeyRightCtrl,
	"r-alt":   golibevdev.KeyRightAlt,
	"ralt":    golibevdev.KeyRightAlt,
	"r-cmd":   golibevdev.KeyRightMeta,
	"rcmd":    golibevdev.KeyRightMeta,
	"r-meta":  golibevdev.KeyRightMeta,
	"rmeta":   golibevdev.KeyRightMeta,
	"r-super": golibevdev.KeyRightMeta,
	"rsuper":  golibevdev.KeyRightMeta,
	"r-shift": golibevdev.KeyRightShift,
	"rshift":  golibevdev.KeyRightShift,
}

func init() {
	for code := golibevdev.KeyReserved + 1; code < golibevdev.KeyMax; code++ {
		name := strings.TrimPrefix(code.String(), "Key")
		name = strings.ToLower(name)
		name = strings.ReplaceAll(name, "_", "-")
		keyMap[name] = code
	}
}

func GetKeyCodes(keys []string) ([]Key, error) {
	keyCodes := make([]Key, 0, len(keys))
	for _, key := range keys {
		c, ok := keyMap[strings.ToLower(key)]
		if !ok {
			return nil, fmt.Errorf("unknown key: %s", key)
		}
		keyCodes = append(keyCodes, c)
	}
	return keyCodes, nil
}

func IsModifier(key golibevdev.KeyEventCode) bool {
	_, ok := Modifiers[key]
	return ok
}
