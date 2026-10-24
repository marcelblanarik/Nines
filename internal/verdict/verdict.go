// Package verdict classifies a computed availability against a target.
package verdict

import (
	"fmt"
	"math"

	"github.com/marcelblanarik/nines/internal/eval"
)

// eps absorbs float noise so that, for example, a model computing exactly 99.95% meets a 99.95% target.
const eps = 1e-12

// headroomShare is the fraction of the error budget that must remain for a plain "achievable".
const headroomShare = 0.2

// meetsConfidence is the P(meets target) needed, when uncertainty ran, for "achievable".
const meetsConfidence = 0.9

// Tiers are the availability tiers a service can be promoted to, ascending (fractions).
var Tiers = []float64{0.99, 0.995, 0.999, 0.9995, 0.9999, 0.99999}

// Input is what the classification looks at.
type Input struct {
	P       float64
	Target  float64
	Ceiling *float64 // nil when nothing is owned
	// PMeets is P(meets target) from the uncertainty run, nil if it did not run.
	PMeets *float64
}

// Classify returns eval.Infeasible, eval.Marginal or eval.Achievable.
func Classify(in Input) string {
	if in.P < in.Target-eps || (in.Ceiling != nil && *in.Ceiling < in.Target-eps) {
		return eval.Infeasible
	}
	if in.PMeets != nil {
		if *in.PMeets < meetsConfidence {
			return eval.Marginal
		}
		return eval.Achievable
	}
	if in.P-in.Target < headroomShare*(1-in.Target) {
		return eval.Marginal
	}
	return eval.Achievable
}

// Tier returns the highest tier at or below the conservative estimate, if any.
func Tier(conservative float64) (float64, bool) {
	best, ok := 0.0, false
	for _, t := range Tiers {
		if conservative >= t-eps {
			best, ok = t, true
		}
	}
	return best, ok
}

// BudgetUsed is the share of the error budget the computed availability consumes.
func BudgetUsed(p, target float64) float64 { return (1 - p) / (1 - target) }

// Recommend writes the one-line advice shown under a check.
func Recommend(r *eval.Result) string {
	tier := "a target below 99%"
	if r.Tier != nil {
		tier = fmt.Sprintf("the %s tier", Percent(*r.Tier))
	}
	switch r.Verdict {
	case eval.Infeasible:
		if r.Ceiling != nil && *r.Ceiling < r.Target-eps {
			return fmt.Sprintf("%s is not possible: even if everything you own were perfect, dependencies cap you at %s. Add redundancy to the dependencies or promise %s.",
				Percent(r.Target), Percent(*r.Ceiling), tier)
		}
		lever := ""
		if len(r.Levers) > 0 {
			lever = fmt.Sprintf(" The biggest lever is %s.", r.Levers[0].Component)
		}
		return fmt.Sprintf("%s is not met (%s computed).%s Improve it, add redundancy, or promise %s.", Percent(r.Target), Percent(r.Availability), lever, tier)
	case eval.Marginal:
		return fmt.Sprintf("%s is met with little room to spare; promise %s or leave more headroom.", Percent(r.Target), tier)
	}
	return fmt.Sprintf("%s is achievable; it uses %.0f%% of the error budget. You could promise %s.", Percent(r.Target), 100*BudgetUsed(r.Availability, r.Target), tier)
}

// Percent formats a fraction as a percentage with three significant digits of unavailability and at
// least two decimals: 98.02%, 99.95%, 99.9099%. Trailing zeros beyond two decimals are dropped.
func Percent(f float64) string {
	decimals := 2
	if u := (1 - f) * 100; u > 0 {
		decimals = min(9, max(2, 2-int(math.Floor(math.Log10(u)))))
	}
	s := fmt.Sprintf("%.*f", decimals, f*100)
	for s[len(s)-1] == '0' && s[len(s)-3] != '.' {
		s = s[:len(s)-1]
	}
	return s + "%"
}
