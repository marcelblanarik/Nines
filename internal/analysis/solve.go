package analysis

import (
	"fmt"

	"github.com/marcelblanarik/nines/internal/compile"
	"github.com/marcelblanarik/nines/internal/eval"
	"github.com/marcelblanarik/nines/internal/model"
)

const solveIterations = 100

// SolveResult answers "how good must this component be?".
type SolveResult struct {
	SchemaVersion int     `json:"schema_version"`
	Component     string  `json:"component"`
	Current       float64 `json:"current"`
	Target        float64 `json:"target"`
	// Required is the smallest availability meeting the target; nil when even a perfect component is not enough.
	Required   *float64 `json:"required"`
	AlreadyMet bool     `json:"already_met"`
	// AtPerfect is the system availability if the component were perfect: the ceiling when Required is nil.
	AtPerfect float64 `json:"at_perfect"`
}

// Solve finds, by binary search on [current, 1], the smallest availability of one component for which
// the system meets the target. P is monotone in every component, so the search is valid.
func Solve(m *model.Model, component string, opt Options) (*SolveResult, error) {
	comp := m.Lookup(component)
	if comp == nil {
		return nil, fmt.Errorf("unknown component %q", component)
	}
	c, err := compile.Compile(m, compile.Options{})
	if err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}
	if len(c.ComponentVars[component]) == 0 {
		return nil, fmt.Errorf("component %q is not used by the model, so changing it changes nothing", component)
	}
	e, err := eval.Build(c, opt.MaxBDDNodes)
	if err != nil {
		return nil, fmt.Errorf("the model is too large to solve exactly: %w", err)
	}
	target := m.Target / 100
	p := func(a float64) float64 { return e.P(c.Probs(map[string]float64{component: a})) }

	cur := comp.Avail.Value()
	res := &SolveResult{SchemaVersion: eval.SchemaVersion, Component: component, Current: cur, Target: target, AtPerfect: p(1)}
	const eps = 1e-12
	switch {
	case p(cur) >= target-eps:
		res.Required, res.AlreadyMet = &cur, true
	case res.AtPerfect >= target-eps:
		lo, hi := cur, 1.0
		for i := 0; i < solveIterations; i++ {
			mid := (lo + hi) / 2
			if p(mid) >= target-eps {
				hi = mid
			} else {
				lo = mid
			}
		}
		res.Required = &hi
	}
	return res, nil
}
