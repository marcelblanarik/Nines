package verdict

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/marcelblanarik/nines/internal/eval"
)

func ptr(f float64) *float64 { return &f }

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		in   Input
		want string
	}{
		{"below target", Input{P: 0.998, Target: 0.999}, eval.Infeasible},
		{"ceiling below target", Input{P: 0.9995, Target: 0.999, Ceiling: ptr(0.9985)}, eval.Infeasible},
		{"exactly target is marginal (no headroom)", Input{P: 0.999, Target: 0.999}, eval.Marginal},
		{"under 20% of the budget as headroom", Input{P: 0.99915, Target: 0.999}, eval.Marginal},
		{"over 20% of the budget as headroom", Input{P: 0.99922, Target: 0.999}, eval.Achievable},
		{"plenty of headroom", Input{P: 0.9999, Target: 0.999}, eval.Achievable},
		{"uncertainty decides, low confidence", Input{P: 0.9999, Target: 0.999, PMeets: ptr(0.85)}, eval.Marginal},
		{"uncertainty decides, high confidence", Input{P: 0.99905, Target: 0.999, PMeets: ptr(0.95)}, eval.Achievable},
		{"float noise at the boundary", Input{P: 0.9995 - 1e-15, Target: 0.9995, PMeets: ptr(1)}, eval.Achievable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { require.Equal(t, c.want, Classify(c.in)) })
	}
}

func TestTier(t *testing.T) {
	for _, c := range []struct {
		p    float64
		want float64
		ok   bool
	}{
		{0.98, 0, false},
		{0.99, 0.99, true},
		{0.9994, 0.999, true},
		{0.9995, 0.9995, true},
		{0.9995 - 1e-14, 0.9995, true},
		{0.99995, 0.9999, true},
		{0.999999, 0.99999, true},
	} {
		got, ok := Tier(c.p)
		require.Equal(t, c.ok, ok, c.p)
		require.Equal(t, c.want, got, c.p)
	}
}

func TestPercent(t *testing.T) {
	for in, want := range map[float64]string{
		0.98018993: "98.02%",
		0.9995:     "99.95%",
		0.999:      "99.90%",
		0.99909988: "99.91%",
		0.99999:    "99.999%",
		0.9999999:  "99.99999%",
		1:          "100.00%",
		0.5:        "50.00%",
		0.972:      "97.20%",
	} {
		require.Equal(t, want, Percent(in), in)
	}
}

func TestRecommend(t *testing.T) {
	r := &eval.Result{Target: 0.9995, Availability: 0.999, Ceiling: ptr(0.9992), Verdict: eval.Infeasible}
	require.Contains(t, Recommend(r), "cap you at 99.92%")
	r = &eval.Result{Target: 0.999, Availability: 0.9999, Verdict: eval.Achievable}
	require.Contains(t, Recommend(r), "achievable")
}
