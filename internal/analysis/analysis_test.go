package analysis

import (
	"math"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/marcelblanarik/nines/internal/eval"
	"github.com/marcelblanarik/nines/internal/model"
)

const checkout = `
service: checkout
target: 99.95
components:
  self:       {availability: 99.98, owned: true}
  auth:       {availability: 99.95, source: measured}
  payments:   {availability: 99.90, source: vendor-sla, transient_fraction: 0.8}
  db-primary: {availability: 99.95, group: az-a}
  db-replica: {availability: 99.95, group: az-b}
  az-a:       {availability: 99.99}
  az-b:       {availability: 99.99}
requires:
  all:
    - self
    - auth
    - retry: {of: payments, attempts: 3}
    - any: [db-primary, db-replica]
`

func parse(t *testing.T, src string) *model.Model {
	t.Helper()
	m, err := model.Parse([]byte(src))
	require.NoError(t, err)
	return m
}

func TestCheckCheckout(t *testing.T) {
	r, err := Check(parse(t, checkout), Options{Seed: 1})
	require.NoError(t, err)
	require.Equal(t, eval.MethodBDD, r.Method)
	spofs := append([]string(nil), r.SPOFs...)
	sort.Strings(spofs)
	require.Equal(t, []string{"auth", "payments", "self"}, spofs)
	require.NotNil(t, r.Ceiling)
	require.Greater(t, *r.Ceiling, r.Availability)
	require.Less(t, r.Availability, r.Target, "auth*self*payments alone is already below 99.95%")
	require.Equal(t, eval.Infeasible, r.Verdict)
	require.NotNil(t, r.Naive)
	require.NotEmpty(t, r.Levers)
	for _, l := range r.Levers {
		require.GreaterOrEqual(t, l.Importance, 0.0)
		require.GreaterOrEqual(t, l.ImprovementPotential, -1e-15)
	}
	require.Equal(t, "auth", r.Levers[0].Component, "auth is a SPOF with no retry cover; payments' transient failures are absorbed by retries")
	require.NotEmpty(t, r.Recommendation)
}

// For a single-variable component the exact gain is Birnbaum importance times unavailability; with a
// transient fraction that product overstates it (retries already absorb the transient failures).
func TestLeverPotential(t *testing.T) {
	r, err := Check(parse(t, checkout), Options{})
	require.NoError(t, err)
	for _, l := range r.Levers {
		product := l.Importance * (1 - l.Availability)
		if l.Component == "payments" {
			require.Less(t, l.ImprovementPotential, product/2)
		} else {
			require.InDelta(t, product, l.ImprovementPotential, 1e-12, l.Component)
		}
	}
}

func TestCutSets(t *testing.T) {
	src := `
service: s
target: 99
components:
  a: {availability: 99}
  b: {availability: 99}
  c: {availability: 99}
  d: {availability: 99}
requires:
  all:
    - a
    - any: [b, c]
    - any: [c, d]
`
	r, err := Check(parse(t, src), Options{})
	require.NoError(t, err)
	require.Equal(t, []string{"a"}, r.SPOFs)
	pairs := map[string]bool{}
	for _, p := range r.CutSets {
		pairs[p[0]+"+"+p[1]] = true
	}
	require.True(t, pairs["b+c"])
	require.True(t, pairs["c+d"])
	require.False(t, pairs["b+d"], "b and d down still leaves c")
}

func TestSolveRoundTrip(t *testing.T) {
	m := parse(t, `
service: s
target: 99.9
components:
  a: {availability: 99.95}
  b: {availability: 99.5}
  c: {availability: 99.99}
requires: {all: [a, b, c]}
`)
	s, err := Solve(m, "b", Options{})
	require.NoError(t, err)
	require.NotNil(t, s.Required)
	require.False(t, s.AlreadyMet)
	m2, err := m.WithAvailability("b", model.PointAvailability(*s.Required*100))
	require.NoError(t, err)
	r, err := Check(m2, Options{})
	require.NoError(t, err)
	require.GreaterOrEqual(t, r.Availability, r.Target-1e-9)
	require.InDelta(t, r.Target, r.Availability, 1e-9)

	m3 := parse(t, `
service: s
target: 99.99
components:
  a: {availability: 99.95}
  b: {availability: 99.5}
requires: {all: [a, b]}
`)
	s, err = Solve(m3, "a", Options{})
	require.NoError(t, err)
	require.Nil(t, s.Required, "even a perfect a cannot rescue a 99.5% b")
	require.InDelta(t, 0.995, s.AtPerfect, 1e-12)

	_, err = Solve(m, "zzz", Options{})
	require.Error(t, err)
}

func TestWhatIf(t *testing.T) {
	m := parse(t, checkout)
	d, err := WhatIf(m, []Set{{"payments", 99.99}}, nil, Options{})
	require.NoError(t, err)
	require.Greater(t, d.After.Availability, d.Before.Availability)
	require.Len(t, d.Rows, 2)
	require.Greater(t, d.Rows[0].DeltaPP, 0.0)
	require.Less(t, d.Rows[0].DeltaDowntimeMin, 0.0)
	d, err = WhatIf(m, nil, []string{"db-replica"}, Options{})
	require.NoError(t, err)
	require.Less(t, d.After.Availability, d.Before.Availability, "losing the redundant replica hurts")
	require.Equal(t, 99.9, m.Lookup("payments").Avail.Percent, "baseline model untouched")
	_, err = WhatIf(m, nil, nil, Options{})
	require.Error(t, err)
}

func TestUncertaintyMeanConverges(t *testing.T) {
	m := parse(t, `
service: s
target: 99.5
components:
  a: {availability: {interval: [99.0, 99.9]}}
  b: {availability: {counts: {good: 9990, total: 10000}}}
  c: {availability: 99.99}
requires: {all: [a, b, c]}
`)
	r, err := Check(m, Options{Uncertainty: true, Seed: 3, Samples: 20000})
	require.NoError(t, err)
	u := r.Uncertainty
	require.NotNil(t, u)
	// P is multilinear in independent components, so E[P] = P(E[a], E[b], c)
	want := ((0.99 + 0.999) / 2) * ((9990.0 + 1) / (10000 + 2)) * 0.9999
	sd := (u.P95 - u.P5) / 3.29 // p5..p95 spans about 3.29 standard deviations
	require.InDelta(t, want, u.Mean, 3*sd/math.Sqrt(float64(u.Samples)))
	require.Less(t, u.P5, u.P50)
	require.Less(t, u.P50, u.P95)

	again, err := Check(m, Options{Uncertainty: true, Seed: 3, Samples: 20000})
	require.NoError(t, err)
	require.Equal(t, r.Uncertainty, again.Uncertainty, "fixed seed gives identical output")
	other, err := Check(m, Options{Uncertainty: true, Seed: 4, Samples: 20000})
	require.NoError(t, err)
	require.NotEqual(t, r.Uncertainty.Mean, other.Uncertainty.Mean)
}

func TestMarginalWithUncertainty(t *testing.T) {
	m := parse(t, `
service: s
target: 99.9
components:
  a: {availability: {interval: [99.85, 99.99]}}
requires: a
`)
	r, err := Check(m, Options{Uncertainty: true, Seed: 1})
	require.NoError(t, err)
	require.Less(t, r.Uncertainty.PMeetsTarget, 0.9)
	require.Equal(t, eval.Marginal, r.Verdict)
}

func TestMonteCarloFallback(t *testing.T) {
	exact, err := Check(parse(t, checkout), Options{})
	require.NoError(t, err)
	opt := Options{MaxBDDNodes: 3, Seed: 9, MCSamples: 300000}
	r, err := Check(parse(t, checkout), opt)
	require.NoError(t, err)
	require.Equal(t, eval.MethodMonteCarlo, r.Method)
	require.NotNil(t, r.AvailabilityCI)
	require.Contains(t, r.Warnings[0], "Monte Carlo")
	se := math.Sqrt(exact.Availability * (1 - exact.Availability) / 300000)
	require.InDelta(t, exact.Availability, r.Availability, 3*se)
	require.Greater(t, r.AvailabilityCI[1], r.AvailabilityCI[0])
	again, err := Check(parse(t, checkout), opt)
	require.NoError(t, err)
	require.Equal(t, r, again)
}

const sharedDB = `
service: s
target: 99
components:
  a: {availability: 90}
  b: {availability: 90}
  db: {availability: 99}
requires:
  all:
    - any:
        - all: [a, db]
        - all: [b, db]
`

func TestExplainSharedMarker(t *testing.T) {
	tr, err := Explain(parse(t, sharedDB), Options{})
	require.NoError(t, err)
	require.InDelta(t, 0.9801, tr.Root.Probability, 1e-12)
	anyNode := tr.Root.Children[0]
	require.Equal(t, []string{"db"}, anyNode.Shared)
	require.InDelta(t, 0.9801, anyNode.Probability, 1e-12)
	require.Empty(t, anyNode.Children[0].Shared)
	require.InDelta(t, 0.891, anyNode.Children[0].Probability, 1e-12)
}

// Symmetric components get the same gain, but its leading digits differ between CPUs. These are the values the Mac
// produced for cdn-a and for cdn-b/cdn-c in testdata/models/operators.yaml: they must count as a tie, so that the
// component name decides the order on every machine.
func TestSameGainIgnoresRounding(t *testing.T) {
	require.True(t, sameGain(1.9832081487036746e-6, 1.9832081485926523e-6))
	require.True(t, sameGain(2.9966013548321513e-7, 2.996601353721928e-7))
	require.True(t, sameGain(0, 0))
	require.False(t, sameGain(1.0e-4, 1.001e-4), "a real difference is not a tie")
	require.False(t, sameGain(0, 1e-9))
}

func TestNaiveComparison(t *testing.T) {
	r, err := Check(parse(t, sharedDB), Options{})
	require.NoError(t, err)
	require.InDelta(t, 0.9801, r.Availability, 1e-12)
	require.InDelta(t, 0.988119, *r.Naive, 1e-12)
}
