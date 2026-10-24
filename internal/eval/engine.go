package eval

import (
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/marcelblanarik/nines/internal/bdd"
	"github.com/marcelblanarik/nines/internal/compile"
)

// DefaultMaxBDDNodes is the node cap before falling back to Monte Carlo.
const DefaultMaxBDDNodes = 2_000_000

// ErrTooLarge means the BDD outgrew the node cap.
var ErrTooLarge = bdd.ErrNodeLimit

// Engine is a compiled model together with its BDD.
type Engine struct {
	C    *compile.Compiled
	M    *bdd.Manager
	Root bdd.Node
	plan *bdd.Plan
	memo map[*compile.Formula]bdd.Node
}

// Build constructs the BDD for c. It returns ErrTooLarge when the cap (0 = default) is exceeded.
func Build(c *compile.Compiled, maxNodes int) (*Engine, error) {
	if maxNodes <= 0 {
		maxNodes = DefaultMaxBDDNodes
	}
	e := &Engine{C: c, M: bdd.New(maxNodes), memo: map[*compile.Formula]bdd.Node{}}
	e.Root = e.Node(c.Root)
	if err := e.M.Err(); err != nil {
		return nil, err
	}
	e.plan = e.M.Plan(e.Root)
	return e, nil
}

// Node returns the BDD of any subformula of the compiled model. Check M.Err afterwards if it may be large.
func (e *Engine) Node(f *compile.Formula) bdd.Node {
	if n, ok := e.memo[f]; ok {
		return n
	}
	var n bdd.Node
	switch f.Kind {
	case compile.FTrue:
		n = bdd.True
	case compile.FFalse:
		n = bdd.False
	case compile.FVar:
		n = e.M.Var(f.Var)
	case compile.FAnd:
		n = bdd.True
		for _, a := range f.Args {
			n = e.M.And(n, e.Node(a))
		}
	case compile.FOr:
		n = bdd.False
		for _, a := range f.Args {
			n = e.M.Or(n, e.Node(a))
		}
	case compile.FKOfN:
		n = e.threshold(f)
	}
	e.memo[f] = n
	return n
}

// threshold builds "at least K of Args" in O(K*n) ITEs: T(j,i) = ITE(arg_i, T(j-1,i+1), T(j,i+1)).
func (e *Engine) threshold(f *compile.Formula) bdd.Node {
	args := make([]bdd.Node, len(f.Args))
	for i, a := range f.Args {
		args[i] = e.Node(a)
	}
	row := make([]bdd.Node, f.K+1) // row[j] = T(j, i+1); starts at i = n where only j = 0 holds
	row[0] = bdd.True
	for j := 1; j <= f.K; j++ {
		row[j] = bdd.False
	}
	for i := len(args) - 1; i >= 0; i-- {
		next := make([]bdd.Node, f.K+1)
		next[0] = bdd.True
		for j := 1; j <= f.K; j++ {
			next[j] = e.M.ITE(args[i], row[j-1], row[j])
		}
		row = next
	}
	return row[f.K]
}

// P is the exact probability the system works, for variable up-probabilities probs.
func (e *Engine) P(probs []float64) float64 { return e.plan.Probability(probs) }

// Nodes is the BDD size.
func (e *Engine) Nodes() int { return e.M.Size() }

// MonteCarlo estimates P(formula) by sampling variable assignments. It returns the estimate and a 95%
// Wilson score interval.
func MonteCarlo(c *compile.Compiled, probs []float64, samples int, rng *rand.Rand) (p float64, lo, hi float64) {
	assign := make([]bool, len(probs))
	ok := 0
	for s := 0; s < samples; s++ {
		for i, q := range probs {
			assign[i] = rng.Float64() < q
		}
		if compile.Eval(c.Root, assign) {
			ok++
		}
	}
	const z = 1.959964
	n := float64(samples)
	p = float64(ok) / n
	centre := (p + z*z/(2*n)) / (1 + z*z/n)
	half := z * math.Sqrt(p*(1-p)/n+z*z/(4*n*n)) / (1 + z*z/n)
	return p, math.Max(0, centre-half), math.Min(1, centre+half)
}

// NewRand returns the package's deterministic generator for a seed.
func NewRand(seed uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
}

func (e *Engine) String() string { return fmt.Sprintf("bdd(%d nodes)", e.Nodes()) }
