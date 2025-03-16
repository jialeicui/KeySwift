package engine

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/buke/quickjs-go"
	"github.com/samber/lo"
)

const (
	FuncGetActiveWindowClass = "getActiveWindowClass"
	FuncGetPressedKeys       = "getPressedKeys"
	FuncGetKeyState          = "getKeyState"
	FuncSendKeys             = "sendKeys"
	FuncOnKeyPress           = "onKeyPress"

	KeySwiftObj = "KeySwift"
)

type Bus interface {
	GetActiveWindowClass() string
	GetPressedKeys() []string
	GetKeyState(key string) string
	SendKeys(keys []string)
}

type Engine struct {
	rt quickjs.Runtime

	byteCode []byte
	script   string

	bus Bus
}

func New(bus Bus, script string) (*Engine, error) {
	rt := quickjs.NewRuntime(
		quickjs.WithExecuteTimeout(30),
		quickjs.WithMemoryLimit(128*1024),
		quickjs.WithGCThreshold(256*1024),
		quickjs.WithMaxStackSize(65534),
		quickjs.WithCanBlock(true),
	)

	ctx := rt.NewContext()
	defer ctx.Close()

	buf, err := ctx.Compile(script)
	if err != nil {
		return nil, err
	}

	e := &Engine{
		rt:       rt,
		byteCode: buf,
		script:   script,

		bus: bus,
	}

	return e, nil
}

func (e *Engine) init(ctx *quickjs.Context) {
	e.registerConsole(ctx)
	e.registerKeySwift(ctx)
}

func (e *Engine) Run() error {
	rt := quickjs.NewRuntime(
		quickjs.WithExecuteTimeout(30),
		quickjs.WithMemoryLimit(128*1024),
		quickjs.WithGCThreshold(256*1024),
		quickjs.WithMaxStackSize(65534),
		quickjs.WithCanBlock(true),
	)
	defer rt.Close()

	ctx := rt.NewContext()
	defer ctx.Close()
	e.init(ctx)
	ret, err := ctx.Eval(e.script)
	if err != nil {
		return err
	}
	ret.Free()
	return nil
}

func (e *Engine) Release() {
	e.rt.Close()
}

func (e *Engine) registerConsole(ctx *quickjs.Context) {
	console := ctx.Object()
	console.Set("log", ctx.Function(func(ctx *quickjs.Context, this quickjs.Value, args []quickjs.Value) quickjs.Value {
		fmt.Printf("%s\n", strings.Join(lo.Map(args, func(v quickjs.Value, _ int) string {
			return v.String()
		}), " "))
		return ctx.Undefined()
	}))
	ctx.Globals().Set("console", console)
}

func (e *Engine) registerKeySwift(ctx *quickjs.Context) {
	keySwift := ctx.Object()
	ctx.Globals().Set(KeySwiftObj, keySwift)

	keySwift.Set(FuncGetActiveWindowClass, ctx.Function(func(ctx *quickjs.Context, this quickjs.Value, args []quickjs.Value) quickjs.Value {
		slog.Debug("GetActiveWindowClass")
		return ctx.String(e.bus.GetActiveWindowClass())
	}))

	keySwift.Set(FuncGetPressedKeys, ctx.Function(func(ctx *quickjs.Context, this quickjs.Value, args []quickjs.Value) quickjs.Value {
		keys := e.bus.GetPressedKeys()
		arr := ctx.Array()
		for i, key := range keys {
			_ = arr.Set(int64(i), ctx.String(key))
		}
		return arr.ToValue()
	}))

	keySwift.Set(FuncGetKeyState, ctx.Function(func(ctx *quickjs.Context, this quickjs.Value, args []quickjs.Value) quickjs.Value {
		key := args[0].String()
		return ctx.String(e.bus.GetKeyState(key))
	}))

	keySwift.Set(FuncSendKeys, ctx.Function(func(ctx *quickjs.Context, this quickjs.Value, args []quickjs.Value) quickjs.Value {
		keys := lo.Map(args, func(v quickjs.Value, _ int) string {
			return v.String()
		})
		slog.Debug("SendKeys", "keys", keys)
		e.bus.SendKeys(keys)
		return ctx.Undefined()
	}))

	keySwift.Set(FuncOnKeyPress, ctx.Function(func(ctx *quickjs.Context, this quickjs.Value, args []quickjs.Value) quickjs.Value {
		slog.Debug("OnKeyPress", "key", args[0].String())
		if len(args) != 2 {
			return ctx.Undefined()
		}

		if !args[0].IsArray() {
			return ctx.Undefined()
		}

		if !args[1].IsFunction() {
			return ctx.Undefined()
		}

		jsKeys := args[0].ToArray()

		expectKeys := make([]string, 0, jsKeys.Len())
		for i := int64(0); i < jsKeys.Len(); i++ {
			item, err := jsKeys.Get(i)
			if err != nil {
				return ctx.Undefined()
			}
			if !item.IsString() {
				return ctx.Undefined()
			}
			expectKeys = append(expectKeys, item.String())
		}

		curKeys := e.bus.GetPressedKeys()

		if a, b := lo.Difference(curKeys, expectKeys); len(a) == 0 && len(b) == 0 {
			ctx.Invoke(args[1], this)
		}

		return ctx.Undefined()
	}))
}
