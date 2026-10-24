// Package analysis runs the questions the CLI and web app ask of a model: check, explain, whatif, solve.
package analysis

import (
	"errors"
	"fmt"
	"sort"

	"github.com/marcelblanarik/nines/internal/bdd"
	"github.com/marcelblanarik/nines/internal/compile"
	"github.com/marcelblanarik/nines/internal/eval"
	"github.com/marcelblanarik/nines/internal/model"
	"github.com/marcelblanarik/nines/internal/verdict"
)

// Defaults.
const (
	DefaultSeed         = uint64(1)
	DefaultSamples      = 10_000
	DefaultMCSamples    = 200_000
	maxCutSetComponents = 60
	maxCutSets          = 20
)

// Options tune a check.
type Options struct {
	// Uncertainty re-evaluates the BDD under sampled availabilities.
	Uncertainty bool
	Seed        uint64
	// Samples is the uncertainty sample count (default 10,000).
	Samples int
	// MaxBDDNodes caps the BDD before falling back to Monte Carlo (default eval.DefaultMaxBDDNodes).
	MaxBDDNodes int
	// MCSamples is the Monte Carlo fallback sample count (default 200,000).
	MCSamples int
}

func (o Options) samples() int {
	if o.Samples <= 0 {
		return DefaultSamples
	}
	return o.Samples
}

func (o Options) mcSamples() int {
	if o.MCSamples <= 0 {
		return DefaultMCSamples
	}
	return o.MCSamples
}

// Check computes the full result for a model.
func Check(m *model.Model, opt Options) (*eval.Result, error) {
	c, err := compile.Compile(m, compile.Options{})
	if err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}
	e, err := eval.Build(c, opt.MaxBDDNodes)
	if errors.Is(err, eval.ErrTooLarge) {
		return checkMonteCarlo(m, c, opt), nil
	}
	if err != nil {
		return nil, err
	}

	target := m.Target / 100
	probs := c.Probs(nil)
	res := &eval.Result{
		SchemaVersion: eval.SchemaVersion,
		Service:       m.Service,
		Target:        target,
		WindowDays:    m.WindowDays,
		Method:        eval.MethodBDD,
		BDDNodes:      e.Nodes(),
		Availability:  e.P(probs),
		Levers:        []eval.Lever{},
		SPOFs:         []string{},
		CutSets:       [][]string{},
		Warnings:      []string{},
	}
	for _, w := range m.Warnings {
		res.Warnings = append(res.Warnings, w.String())
	}

	res.Naive = naive(m, opt, res)
	res.Ceiling = ceiling(m, c, e)
	res.Levers = levers(m, c, e, probs, res.Availability)
	res.SPOFs, res.CutSets, res.CutSetsTruncated = cutSets(m, c, e, &res.Warnings)
	for _, s := range res.SPOFs {
		if a := m.Lookup(s).Avail.Value(); a < target {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s is a single point of failure at %s, below the %s target", s, verdict.Percent(a), verdict.Percent(target)))
		}
	}

	in := verdict.Input{P: res.Availability, Target: target, Ceiling: res.Ceiling}
	conservative := res.Availability
	if opt.Uncertainty {
		res.Uncertainty = uncertainty(c, e, opt, target)
		in.PMeets, conservative = &res.Uncertainty.PMeetsTarget, res.Uncertainty.P5
		if !hasUncertainInput(m) {
			res.Warnings = append(res.Warnings, "no component has an interval or counts, so the uncertainty run has nothing to vary; enter vendor SLAs as intervals with a pessimistic lower end")
		}
	}
	res.Verdict = verdict.Classify(in)
	if t, ok := verdict.Tier(conservative); ok {
		res.Tier = &t
	}
	res.Recommendation = verdict.Recommend(res)
	return res, nil
}

// naive evaluates the model with every occurrence of a shared component made independent.
func naive(m *model.Model, opt Options, res *eval.Result) *float64 {
	nc, err := compile.Compile(m, compile.Options{Naive: true})
	if err != nil {
		return nil
	}
	ne, err := eval.Build(nc, opt.MaxBDDNodes)
	if err != nil {
		res.Warnings = append(res.Warnings, "naive comparison skipped: "+err.Error())
		return nil
	}
	p := ne.P(nc.Probs(nil))
	return &p
}

func hasUncertainInput(m *model.Model) bool {
	for _, c := range m.Components {
		if c.Avail.Kind != model.Point {
			return true
		}
	}
	return false
}

func ownedAvail(m *model.Model) (map[string]float64, bool) {
	out := map[string]float64{}
	for _, c := range m.Components {
		if c.Owned {
			out[c.Name] = 1
		}
	}
	return out, len(out) > 0
}

func ceiling(m *model.Model, c *compile.Compiled, e *eval.Engine) *float64 {
	owned, has := ownedAvail(m)
	if !has {
		return nil
	}
	p := e.P(c.Probs(owned))
	return &p
}

// withVars returns a copy of probs with the given variables forced to p.
func withVars(probs []float64, vars []int, p float64) []float64 {
	out := append([]float64(nil), probs...)
	for _, v := range vars {
		out[v] = p
	}
	return out
}

func levers(m *model.Model, c *compile.Compiled, e *eval.Engine, probs []float64, base float64) []eval.Lever {
	out := make([]eval.Lever, 0, len(c.Components))
	for _, name := range c.Components {
		vars := c.ComponentVars[name]
		perfect := e.P(withVars(probs, vars, 1))
		failed := e.P(withVars(probs, vars, 0))
		comp := m.Lookup(name)
		a := comp.Avail.Value()
		out = append(out, eval.Lever{
			Component:            name,
			Availability:         a,
			Owned:                comp.Owned,
			Importance:           perfect - failed,
			ImprovementPotential: perfect - base,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ImprovementPotential != out[j].ImprovementPotential {
			return out[i].ImprovementPotential > out[j].ImprovementPotential
		}
		return out[i].Component < out[j].Component
	})
	return out
}

// cutSets finds single points of failure and, for up to 60 components, the order-2 cut sets.
func cutSets(m *model.Model, c *compile.Compiled, e *eval.Engine, warnings *[]string) (spofs []string, pairs [][]string, truncated bool) {
	spofs, pairs = []string{}, [][]string{}
	failed := func(names ...string) bool {
		assign := map[int]bool{}
		for _, n := range names {
			for _, v := range c.ComponentVars[n] {
				assign[v] = false
			}
		}
		return e.M.Restrict(e.Root, assign) == bdd.False
	}
	isSPOF := map[string]bool{}
	for _, name := range c.Components {
		if failed(name) {
			isSPOF[name] = true
			spofs = append(spofs, name)
		}
	}
	unavail := func(n string) float64 { return 1 - m.Lookup(n).Avail.Value() }
	sort.SliceStable(spofs, func(i, j int) bool {
		if unavail(spofs[i]) != unavail(spofs[j]) {
			return unavail(spofs[i]) > unavail(spofs[j])
		}
		return spofs[i] < spofs[j]
	})
	if len(c.Components) > maxCutSetComponents {
		*warnings = append(*warnings, fmt.Sprintf("order-2 cut sets skipped: more than %d components", maxCutSetComponents))
		return spofs, pairs, false
	}
	var rest []string
	for _, n := range c.Components {
		if !isSPOF[n] {
			rest = append(rest, n)
		}
	}
	type pair struct {
		a, b  string
		joint float64
	}
	var found []pair
	for i := range rest {
		for j := i + 1; j < len(rest); j++ {
			if failed(rest[i], rest[j]) {
				found = append(found, pair{rest[i], rest[j], unavail(rest[i]) * unavail(rest[j])})
			}
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].joint != found[j].joint {
			return found[i].joint > found[j].joint
		}
		return found[i].a+found[i].b < found[j].a+found[j].b
	})
	if len(found) > maxCutSets {
		found, truncated = found[:maxCutSets], true
	}
	for _, p := range found {
		pairs = append(pairs, []string{p.a, p.b})
	}
	return spofs, pairs, truncated
}

// checkMonteCarlo is the fallback when the BDD outgrows its cap: only the headline numbers survive.
func checkMonteCarlo(m *model.Model, c *compile.Compiled, opt Options) *eval.Result {
	target := m.Target / 100
	n := opt.mcSamples()
	p, lo, hi := eval.MonteCarlo(c, c.Probs(nil), n, eval.NewRand(opt.Seed))
	res := &eval.Result{
		SchemaVersion:  eval.SchemaVersion,
		Service:        m.Service,
		Target:         target,
		WindowDays:     m.WindowDays,
		Method:         eval.MethodMonteCarlo,
		Availability:   p,
		AvailabilityCI: &[2]float64{lo, hi},
		Levers:         []eval.Lever{},
		SPOFs:          []string{},
		CutSets:        [][]string{},
		Warnings: []string{fmt.Sprintf("the exact BDD exceeded the node cap, so the result is a Monte Carlo estimate from %d samples (95%% interval %s to %s); levers, single points of failure and uncertainty are unavailable",
			n, verdict.Percent(lo), verdict.Percent(hi))},
	}
	for _, w := range m.Warnings {
		res.Warnings = append(res.Warnings, w.String())
	}
	if owned, has := ownedAvail(m); has {
		cp, _, _ := eval.MonteCarlo(c, c.Probs(owned), n, eval.NewRand(opt.Seed))
		res.Ceiling = &cp
	}
	res.Verdict = verdict.Classify(verdict.Input{P: p, Target: target, Ceiling: res.Ceiling})
	if t, ok := verdict.Tier(lo); ok {
		res.Tier = &t
	}
	res.Recommendation = verdict.Recommend(res)
	return res
}

// Explain builds the derivation trace.
func Explain(m *model.Model, opt Options) (*eval.Trace, error) {
	c, err := compile.Compile(m, compile.Options{})
	if err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}
	e, err := eval.Build(c, opt.MaxBDDNodes)
	if err != nil {
		return nil, fmt.Errorf("the model is too large to explain exactly: %w", err)
	}
	return e.Trace(c.Probs(nil))
}
