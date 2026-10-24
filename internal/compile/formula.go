// Package compile turns a model's requires tree into a boolean formula over independent variables.
package compile

// FKind is the type of a formula node.
type FKind uint8

const (
	FTrue FKind = iota
	FFalse
	FVar
	FAnd
	FOr
	FKOfN // at least K of Args
)

// Formula is an immutable boolean formula. Subformulas may be shared.
type Formula struct {
	Kind FKind
	Var  int
	K    int
	Args []*Formula
}

// Eval evaluates f under a full assignment of variables.
func Eval(f *Formula, assign []bool) bool {
	switch f.Kind {
	case FTrue:
		return true
	case FFalse:
		return false
	case FVar:
		return assign[f.Var]
	case FAnd:
		for _, a := range f.Args {
			if !Eval(a, assign) {
				return false
			}
		}
		return true
	case FOr:
		for _, a := range f.Args {
			if Eval(a, assign) {
				return true
			}
		}
		return false
	default:
		n := 0
		for _, a := range f.Args {
			if Eval(a, assign) {
				n++
			}
		}
		return n >= f.K
	}
}

// Support is the set of variables f mentions.
func Support(f *Formula) map[int]struct{} {
	out := map[int]struct{}{}
	seen := map[*Formula]bool{}
	var rec func(*Formula)
	rec = func(g *Formula) {
		if seen[g] {
			return
		}
		seen[g] = true
		if g.Kind == FVar {
			out[g.Var] = struct{}{}
		}
		for _, a := range g.Args {
			rec(a)
		}
	}
	rec(f)
	return out
}

func v(i int) *Formula { return &Formula{Kind: FVar, Var: i} }

func and(args ...*Formula) *Formula {
	if len(args) == 1 {
		return args[0]
	}
	return &Formula{Kind: FAnd, Args: args}
}

func or(args ...*Formula) *Formula {
	if len(args) == 1 {
		return args[0]
	}
	return &Formula{Kind: FOr, Args: args}
}
