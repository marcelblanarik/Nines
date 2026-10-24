package eval_test

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/marcelblanarik/nines/internal/compile"
	"github.com/marcelblanarik/nines/internal/eval"
	"github.com/marcelblanarik/nines/internal/model"
)

func prob(t *testing.T, src string, naive bool) float64 {
	t.Helper()
	m, err := model.Parse([]byte(src))
	require.NoError(t, err)
	c, err := compile.Compile(m, compile.Options{Naive: naive})
	require.NoError(t, err)
	e, err := eval.Build(c, 0)
	require.NoError(t, err)
	return e.P(c.Probs(nil))
}

func header(comps, requires string) string {
	return "service: s\ntarget: 99\ncomponents:\n" + comps + "requires:\n" + requires
}

func TestGolden(t *testing.T) {
	bridge := header(`  A: {availability: 90}
  B: {availability: 90}
  C: {availability: 90}
  D: {availability: 90}
  E: {availability: 90}
`, `  any:
    - all: [A, D]
    - all: [B, E]
    - all: [A, C, E]
    - all: [B, C, D]
`)
	shared := header(`  a: {availability: 90}
  b: {availability: 90}
  db: {availability: 99}
`, `  all:
    - any:
        - all: [a, db]
        - all: [b, db]
`)
	fan := "service: s\ntarget: 99\ncomponents:\n"
	var names []string
	for i := 0; i < 20; i++ {
		fan += fmt.Sprintf("  d%d: {availability: 99.9}\n", i)
		names = append(names, fmt.Sprintf("d%d", i))
	}
	fan += "requires: {all: [" + strings.Join(names, ", ") + "]}\n"

	hard := 1 - 0.2*0.001
	q := 0.0008 / hard
	cases := []struct {
		name string
		src  string
		want float64
	}{
		{"series", header("  a: {availability: 99.9}\n  b: {availability: 99.9}\n", "  all: [a, b]\n"), 0.998001},
		{"parallel", header("  a: {availability: 99}\n  b: {availability: 99}\n", "  any: [a, b]\n"), 0.9999},
		{"2-of-3", header("  a: {availability: 90}\n  b: {availability: 90}\n  c: {availability: 90}\n", "  k_of_n: {k: 2, of: [a, b, c]}\n"), 0.972},
		{"shared", shared, 0.9801},
		{"bridge", bridge, 0.97848},
		{"fan-out", fan, math.Pow(0.999, 20)},
		{"retry", header("  a: {availability: 99.9, transient_fraction: 0.8}\n", "  retry: {of: a, attempts: 3}\n"), hard * (1 - q*q*q)},
		{"retry no transient", header("  a: {availability: 99.9}\n", "  retry: {of: a, attempts: 3}\n"), 0.999},
		{"single call with transient", header("  a: {availability: 99.9, transient_fraction: 0.8}\n", "  a\n"), 0.999},
		{"fallback", header("  p: {availability: 99}\n  f: {availability: 99}\n", "  fallback: {primary: p, fallback: f, coverage: 0.5}\n"), 0.99495},
		{"cached", header("  o: {availability: 99}\n", "  cached: {of: o, hit_ratio: 0.9}\n"), 0.999},
		{"optional", header("  o: {availability: 99}\n", "  optional: {of: o, touch: 0.5}\n"), 0.995},
		{"group", header("  x: {availability: 99, group: g}\n  y: {availability: 99, group: g}\n  g: {availability: 98}\n", "  all: [x, y]\n"), 0.99 * 0.99 * 0.98},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.InDelta(t, c.want, prob(t, c.src, false), 1e-12)
		})
	}
	t.Run("naive shared", func(t *testing.T) {
		require.InDelta(t, 0.988119, prob(t, shared, true), 1e-12)
	})
	t.Run("naive group", func(t *testing.T) {
		src := header("  x: {availability: 99, group: g}\n  y: {availability: 99, group: g}\n  g: {availability: 98}\n", "  all: [x, y]\n")
		require.InDelta(t, 0.99*0.99*0.98*0.98, prob(t, src, true), 1e-12)
	})
}

// randFormula builds a random formula over n variables.
func randFormula(rng *rand.Rand, n, depth int) *compile.Formula {
	if depth == 0 || rng.IntN(4) == 0 {
		return &compile.Formula{Kind: compile.FVar, Var: rng.IntN(n)}
	}
	k := 2 + rng.IntN(3)
	args := make([]*compile.Formula, k)
	for i := range args {
		args[i] = randFormula(rng, n, depth-1)
	}
	switch rng.IntN(3) {
	case 0:
		return &compile.Formula{Kind: compile.FAnd, Args: args}
	case 1:
		return &compile.Formula{Kind: compile.FOr, Args: args}
	}
	return &compile.Formula{Kind: compile.FKOfN, K: 1 + rng.IntN(k), Args: args}
}

func TestBruteForceOracle(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 43))
	for trial := 0; trial < 300; trial++ {
		n := 1 + rng.IntN(15)
		f := randFormula(rng, n, 4)
		c := &compile.Compiled{Root: f, Vars: make([]compile.Var, n)}
		e, err := eval.Build(c, 0)
		require.NoError(t, err)
		probs := make([]float64, n)
		for i := range probs {
			probs[i] = rng.Float64()
		}
		want := 0.0
		a := make([]bool, n)
		for mask := 0; mask < 1<<n; mask++ {
			w := 1.0
			for i := range a {
				a[i] = mask>>i&1 == 1
				if a[i] {
					w *= probs[i]
				} else {
					w *= 1 - probs[i]
				}
			}
			if compile.Eval(f, a) {
				want += w
			}
		}
		require.InDelta(t, want, e.P(probs), 1e-12, "trial %d", trial)
	}
}

func TestMonotone(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for trial := 0; trial < 100; trial++ {
		n := 2 + rng.IntN(8)
		f := randFormula(rng, n, 4)
		e, err := eval.Build(&compile.Compiled{Root: f, Vars: make([]compile.Var, n)}, 0)
		require.NoError(t, err)
		probs := make([]float64, n)
		for i := range probs {
			probs[i] = rng.Float64()
		}
		base := e.P(probs)
		for i := range probs {
			old := probs[i]
			probs[i] = math.Min(1, old+0.1)
			require.GreaterOrEqual(t, e.P(probs)+1e-12, base)
			probs[i] = old
		}
	}
}

func TestMonteCarloConverges(t *testing.T) {
	src := header("  a: {availability: 90}\n  b: {availability: 90}\n  db: {availability: 99}\n",
		"  all:\n    - any:\n        - all: [a, db]\n        - all: [b, db]\n")
	m, err := model.Parse([]byte(src))
	require.NoError(t, err)
	c, err := compile.Compile(m, compile.Options{})
	require.NoError(t, err)
	probs := c.Probs(nil)
	const n = 200000
	p, lo, hi := eval.MonteCarlo(c, probs, n, eval.NewRand(5))
	require.InDelta(t, 0.9801, p, 3*math.Sqrt(0.9801*0.0199/n))
	require.Less(t, lo, 0.9801)
	require.Greater(t, hi, 0.9801)
	p2, _, _ := eval.MonteCarlo(c, probs, n, eval.NewRand(5))
	require.Equal(t, p, p2, "fixed seed is reproducible")
}

func TestNodeLimit(t *testing.T) {
	n := 30
	args := make([]*compile.Formula, n)
	for i := range args {
		args[i] = &compile.Formula{Kind: compile.FVar, Var: i}
	}
	f := &compile.Formula{Kind: compile.FKOfN, K: n / 2, Args: args}
	_, err := eval.Build(&compile.Compiled{Root: f, Vars: make([]compile.Var, n)}, 20)
	require.ErrorIs(t, err, eval.ErrTooLarge)
}
