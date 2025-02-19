package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/jialeicui/golibevdev"
	"github.com/samber/lo"

	"github.com/jialeicui/keyswift/pkg/conf"
	"github.com/jialeicui/keyswift/pkg/evdev"
	"github.com/jialeicui/keyswift/pkg/keys"
	"github.com/jialeicui/keyswift/pkg/wininfo"
	"github.com/jialeicui/keyswift/pkg/wininfo/dbus"
)

var (
	flagKeyboard = flag.String("keyboard", "Apple", "")
)

func main() {
	c, err := conf.LoadConfig(conf.DefaultConfigPath())
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Loaded config: %+v\n", c)

	r, err := dbus.New()
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()

	fmt.Println("Window Monitor service is running...")

	r.OnActiveWindowChange(func(info *wininfo.WinInfo) {
		fmt.Printf("Active window: %s - %s\n", info.Title, info.Class)
	})

	devs, err := evdev.NewOverviewImpl().ListInputDevices()
	if err != nil {
		log.Fatal(err)
	}

	dev, ok := lo.Find(devs, func(item *evdev.InputDevice) bool {
		return strings.Contains(item.Name, *flagKeyboard)
	})
	if !ok {
		log.Fatalf("Keyboard %s not found", *flagKeyboard)
	}

	out, err := golibevdev.NewVirtualKeyboard("keyswift")
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	in, err := golibevdev.NewInputDev(dev.Path)
	if err != nil {
		log.Fatal(err)
	}
	defer in.Close()

	gnome := keys.NewGnomeKeys(out)

	for {
		ev, err := in.NextEvent(golibevdev.ReadFlagNormal)
		if err != nil {
			log.Fatal(err)
		}
		if ev.Type != golibevdev.EvKey {
			continue
		}
		if ev.Code == golibevdev.KeyC {
			gnome.Copy()
		}
	}
}
