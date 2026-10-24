package bdd

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"
)

// fn is a hand-written boolean function used as the oracle for the BDD.
type fn func(a []bool) bool

func buildAndOracle(rng *rand.Rand, m *Manager, n, depth int) (Node, fn) {
	if depth == 0 || rng.IntN(4) == 0 {
		v := rng.IntN(n)
		return m.Var(v), func(a []bool) bool { return a[v] }
	}
	f, ff := buildAndOracle(rng, m, n, depth-1)
	g, gg := buildAndOracle(rng, m, n, depth-1)
	switch rng.IntN(3) {
	case 0:
		return m.And(f, g), func(a []bool) bool { return ff(a) && gg(a) }
	case 1:
		return m.Or(f, g), func(a []bool) bool { return ff(a) || gg(a) }
	default:
		h, hh := buildAndOracle(rng, m, n, depth-1)
		return m.ITE(f, g, h), func(a []bool) bool {
			if ff(a) {
				return gg(a)
			}
			return hh(a)
		}
	}
}

func TestProbabilityMatchesBruteForce(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 8))
	for trial := 0; trial < 200; trial++ {
		n := 1 + rng.IntN(12)
		m := New(0)
		root, oracle := buildAndOracle(rng, m, n, 5)
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
			if oracle(a) {
				want += w
			}
		}
		require.InDelta(t, want, m.Probability(root, probs), 1e-12)
	}
}

func TestRestrict(t *testing.T) {
	m := New(0)
	x, y := m.Var(0), m.Var(1)
	f := m.And(x, y)
	require.Equal(t, False, m.Restrict(f, map[int]bool{0: false}))
	require.Equal(t, y, m.Restrict(f, map[int]bool{0: true}))
	require.Equal(t, True, m.Restrict(m.Or(x, y), map[int]bool{1: true}))
	require.Equal(t, []int{0, 1}, m.Support(f))
}

func TestTerminalProbability(t *testing.T) {
	m := New(0)
	require.Equal(t, 0.0, m.Probability(False, nil))
	require.Equal(t, 1.0, m.Probability(True, nil))
}

func TestNodeLimit(t *testing.T) {
	m := New(10)
	f := True
	for i := 0; i < 20; i++ {
		f = m.And(f, m.Var(i))
	}
	require.ErrorIs(t, m.Err(), ErrNodeLimit)
}

func TestCanonical(t *testing.T) {
	m := New(0)
	a, b := m.Var(0), m.Var(1)
	require.Equal(t, m.Or(a, b), m.Or(b, a))
	require.Equal(t, m.And(a, m.Or(a, b)), a, "absorption reduces to the same node")
	require.False(t, math.IsNaN(m.Probability(a, []float64{0.3})))
}
