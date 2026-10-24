package report

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/marcelblanarik/nines/internal/analysis"
	"github.com/marcelblanarik/nines/internal/eval"
)

// Trace renders the derivation as an indented outline in plain words.
func Trace(t *eval.Trace) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s works %s of the time because:\n\n", t.Service, Percent(t.Root.Probability))
	writeTrace(&b, t.Root, 0)
	return b.String()
}

func writeTrace(b *strings.Builder, n *eval.TraceNode, depth int) {
	indent := strings.Repeat("  ", depth)
	fmt.Fprintf(b, "%s- %s: %s\n", indent, n.Label, Percent(n.Probability))
	if len(n.Shared) > 0 {
		fmt.Fprintf(b, "%s  (shared components: %s. The parts below are not independent, so this is the exact value, not a product of them.)\n",
			indent, strings.Join(n.Shared, ", "))
	}
	for _, c := range n.Children {
		writeTrace(b, c, depth+1)
	}
}

// Diff renders a what-if as a before/after table.
func Diff(d *analysis.Diff) string {
	var b strings.Builder
	for _, c := range d.Changes {
		fmt.Fprintf(&b, "change: %s\n", c)
	}
	days := d.Before.WindowDays
	fmt.Fprintf(&b, "\n  %-10s %-12s %-12s %-14s %s\n", "", "before", "after", "change", fmt.Sprintf("downtime change / %dd", days))
	for _, r := range d.Rows {
		fmt.Fprintf(&b, "  %-10s %-12s %-12s %-14s %s\n", r.Label, Percent(r.Before), Percent(r.After),
			fmt.Sprintf("%+.4f pp", r.DeltaPP), signedMinutes(r.DeltaDowntimeMin))
	}
	fmt.Fprintf(&b, "  %-10s %-12s %-12s\n", "verdict", d.Before.Verdict, d.After.Verdict)
	for _, w := range d.After.Warnings {
		fmt.Fprintf(&b, "warning: %s\n", w)
	}
	return b.String()
}

func signedMinutes(m float64) string {
	sign := "+"
	if m < 0 {
		sign = "-"
	}
	return sign + minutes(math.Abs(m))
}

// Solve renders the answer to "how good must this component be?".
func Solve(s *analysis.SolveResult) string {
	switch {
	case s.Required == nil:
		return fmt.Sprintf("Even a perfect %s is not enough for %s: the system would reach %s. That is the ceiling set by everything else.\n",
			s.Component, Percent(s.Target), Percent(s.AtPerfect))
	case s.AlreadyMet:
		return fmt.Sprintf("%s at %s already lets the system meet %s.\n", s.Component, Percent(s.Current), Percent(s.Target))
	}
	// Round up so that feeding the printed number back into the model still meets the target.
	up := math.Ceil(*s.Required*100*1e8) / 1e8
	return fmt.Sprintf("%s must be at least %s%% (now %s) for the system to meet %s.\n",
		s.Component, strconv.FormatFloat(up, 'f', -1, 64), Percent(s.Current), Percent(s.Target))
}
