package mode

import (
	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

var _ Expr = (*ExprImpl)(nil)

type Expr interface {
	Test(*Env) bool
}

type Env struct {
	data map[string]any
}

func NewEnv() *Env {
	return &Env{data: make(map[string]any)}
}

func (e *Env) Set(key string, value any) {
	e.data[key] = value
}

type ExprImpl struct {
	p *vm.Program
}

func (e ExprImpl) Test(m *Env) bool {
	r, err := expr.Run(e.p, m.data)
	if err != nil {
		return false
	}
	switch v := r.(type) {
	case bool:
		return v
	case float64:
		return v != 0
	case int:
		return v != 0
	case string:
		return v != ""
	default:
		return false
	}
}

type TrueExpr struct{}

func (TrueExpr) Test(*Env) bool {
	return true
}

func NewTrueExpr() *TrueExpr {
	return &TrueExpr{}
}
