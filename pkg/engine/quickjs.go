package engine

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/buke/quickjs-go"
	"github.com/jialeicui/golibevdev"
	"github.com/samber/lo"

	"github.com/jialeicui/keyswift/pkg/keys"
	"github.com/jialeicui/keyswift/pkg/utils/cache"
)

var _ Engine = (*QuickJS)(nil)

const maxPressed = 16

type QuickJS struct {
	rt quickjs.Runtime

	byteCode []byte
	script   string

	init      bool
	keysWatch map[[maxPressed]golibevdev.KeyEventCode]struct{}

	keyCache cache.Cache[string, []keys.Key]
}

func newJsRuntime() quickjs.Runtime {
	return quickjs.NewRuntime(
		quickjs.WithExecuteTimeout(0),
		quickjs.WithMemoryLimit(1280*1024),
		quickjs.WithGCThreshold(2560*1024),
		quickjs.WithMaxStackSize(65534),
		quickjs.WithCanBlock(true),
	)
}

func NewQuickJS(script string) (*QuickJS, error) {
	rt := newJsRuntime()

	defer rt.Close()

	ctx := rt.NewContext()
	defer ctx.Close()

	buf, err := ctx.Compile(script)
	if err != nil {
		return nil, err
	}

	e := &QuickJS{
		rt:       rt,
		byteCode: buf,
		script:   script,

		init:      false,
		keysWatch: map[[maxPressed]golibevdev.KeyEventCode]struct{}{},
		keyCache:  cache.New[string, []keys.Key](),
	}

	return e, nil
}

func (e *QuickJS) Run(session Bus) error {
	if e.fastIgnore(session) {
		return nil
	}

	rt := newJsRuntime()
	defer rt.Close()

	ctx := rt.NewContext()
	defer ctx.Close()

	e.registerConsole(ctx)
	e.registerKeySwift(ctx, session)

	ret, err := ctx.EvalBytecode(e.byteCode)
	if err != nil {
		return err
	}
	ret.Free()
	e.init = true
	return nil
}

func (e *QuickJS) Release() {
	e.rt.Close()
}

func (e *QuickJS) fastIgnore(session Bus) bool {
	if !e.init {
		return false
	}

	pressed := session.GetPressedKeys()
	slices.Sort(pressed)

	k := [maxPressed]golibevdev.KeyEventCode{}
	copy(k[:], pressed)
	_, ok := e.keysWatch[k]
	slog.Debug("fastIgnore", "keys", pressed, "ok", !ok)
	return !ok
}

func (e *QuickJS) registerConsole(ctx *quickjs.Context) {
	console := ctx.Object()
	console.Set("log", ctx.Function(func(ctx *quickjs.Context, this quickjs.Value, args []quickjs.Value) quickjs.Value {
		fmt.Printf("%s\n", strings.Join(lo.Map(args, func(v quickjs.Value, _ int) string {
			return v.String()
		}), " "))
		return ctx.Undefined()
	}))
	ctx.Globals().Set("console", console)
}

func (e *QuickJS) registerKeySwift(ctx *quickjs.Context, session Bus) {
	keySwift := ctx.Object()
	ctx.Globals().Set(KeySwiftObj, keySwift)

	keySwift.Set(FuncGetActiveWindowClass, ctx.Function(func(ctx *quickjs.Context, this quickjs.Value, args []quickjs.Value) quickjs.Value {
		return ctx.String(session.GetActiveWindowClass())
	}))

	keySwift.Set(FuncSendKeys, ctx.Function(func(ctx *quickjs.Context, this quickjs.Value, args []quickjs.Value) quickjs.Value {
		if len(args) != 1 {
			return ctx.Undefined()
		}

		if !args[0].IsArray() {
			return ctx.Undefined()
		}

		jsKeys := args[0].ToArray()
		keyStrArr := make([]string, 0, jsKeys.Len())
		for i := int64(0); i < jsKeys.Len(); i++ {
			item, err := jsKeys.Get(i)
			if err != nil {
				return ctx.Undefined()
			}
			keyStrArr = append(keyStrArr, item.String())
		}
		slices.Sort(keyStrArr)

		keyCodes, err := e.keyCache.Get(strings.Join(keyStrArr, ","), func() ([]keys.Key, error) {
			return keys.GetKeyCodes(keyStrArr)
		})

		if err != nil {
			slog.Error("failed to get key codes", "error", err)
			return ctx.Undefined()
		}

		session.SendKeys(keyCodes)

		return ctx.Undefined()
	}))

	keySwift.Set(FuncOnKeyPress, ctx.Function(func(ctx *quickjs.Context, this quickjs.Value, args []quickjs.Value) quickjs.Value {
		if len(args) != 2 {
			slog.Error("OnKeyPress requires two arguments")
			return ctx.Undefined()
		}

		if !args[0].IsArray() {
			slog.Error("keys must be an array")
			return ctx.Undefined()
		}

		if !args[1].IsFunction() {
			slog.Error("OnKeyPress requires a function as the second argument")
			return ctx.Undefined()
		}

		jsKeys := args[0].ToArray()

		keyStrArr := make([]string, 0, jsKeys.Len())
		for i := int64(0); i < jsKeys.Len(); i++ {
			item, err := jsKeys.Get(i)
			if err != nil {
				slog.Error("failed to get key by index", "error", err, "index", i)
				return ctx.Undefined()
			}
			if !item.IsString() {
				slog.Error("key is not a string", "key", item.String())
				return ctx.Undefined()
			}
			// TODO: if modifier key position is not fixed, we need to handle it
			keyStrArr = append(keyStrArr, item.String())
		}

		slices.Sort(keyStrArr)
		expected, err := e.keyCache.Get(strings.Join(keyStrArr, ","), func() ([]keys.Key, error) {
			return keys.GetKeyCodes(keyStrArr)
		})
		if err != nil {
			slog.Error("failed to get key codes", "error", err)
			return ctx.Undefined()
		}

		if !e.init {
			slices.Sort(expected)
			slog.Debug("add keys watch", "keys", keyStrArr, "codes", expected)
			k := [maxPressed]golibevdev.KeyEventCode{}
			copy(k[:], expected)
			e.keysWatch[k] = struct{}{}
		}

		curKeys := session.GetPressedKeys()

		if a, b := lo.Difference(curKeys, expected); len(a) == 0 && len(b) == 0 {
			ctx.Invoke(args[1], this)
		}

		return ctx.Undefined()
	}))
}
