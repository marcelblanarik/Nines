// Package report renders results as text, JSON, Markdown and Mermaid.
package report

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/marcelblanarik/nines/internal/eval"
	"github.com/marcelblanarik/nines/internal/verdict"
)

// Formats accepted by Check.
const (
	FormatTable = "table"
	FormatJSON  = "json"
	FormatMD    = "md"
)

const (
	topLevers     = 5
	maxSPOFsShown = 10
	// naiveDiffers is how far naive must be from exact before the comparison is worth showing.
	naiveDiffers = 1e-12
)

// Percent formats a fraction as a percentage.
func Percent(f float64) string { return verdict.Percent(f) }

// Downtime formats the downtime a fractional availability allows per window.
func Downtime(avail float64, days int) string {
	return minutes((1 - avail) * float64(days) * 24 * 60)
}

func minutes(m float64) string {
	switch {
	case m >= 100:
		return fmt.Sprintf("%.0f min", m)
	case m >= 1:
		return fmt.Sprintf("%.1f min", m)
	}
	return fmt.Sprintf("%.2f min", m)
}

// JSON renders any result type with stable indentation.
func JSON(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}

// Check renders a check result in the requested format.
func Check(r *eval.Result, format string) (string, error) {
	switch format {
	case FormatTable, "":
		return checkTable(r), nil
	case FormatJSON:
		return JSON(r)
	case FormatMD:
		return checkMarkdown(r), nil
	}
	return "", fmt.Errorf("unknown format %q (use table, json or md)", format)
}

func showNaive(r *eval.Result) bool {
	return r.Naive != nil && math.Abs(*r.Naive-r.Availability) > naiveDiffers
}

func computed(r *eval.Result) string {
	s := Percent(r.Availability)
	if r.AvailabilityCI != nil {
		s += fmt.Sprintf(" (95%% interval %s to %s)", Percent(r.AvailabilityCI[0]), Percent(r.AvailabilityCI[1]))
	}
	return s
}

func tierText(r *eval.Result) string {
	if r.Tier == nil {
		return "below 99%"
	}
	return Percent(*r.Tier)
}

func checkTable(r *eval.Result) string {
	var b strings.Builder
	row := func(label, value string) { fmt.Fprintf(&b, "  %-18s %s\n", label, value) }
	fmt.Fprintf(&b, "%s\n\n", r.Service)
	row("Target", fmt.Sprintf("%s  (%s downtime / %dd)", Percent(r.Target), Downtime(r.Target, r.WindowDays), r.WindowDays))
	row("Computed", fmt.Sprintf("%s  (%s downtime / %dd)", computed(r), Downtime(r.Availability, r.WindowDays), r.WindowDays))
	if showNaive(r) {
		row("", fmt.Sprintf("what naive multiplication would say: %s", Percent(*r.Naive)))
	}
	if r.Ceiling != nil {
		row("Dependency ceiling", fmt.Sprintf("%s  (%s downtime / %dd, if everything you own were perfect)", Percent(*r.Ceiling), Downtime(*r.Ceiling, r.WindowDays), r.WindowDays))
	} else {
		row("Dependency ceiling", "not computed: no component is marked owned: true")
	}
	row("Verdict", strings.ToUpper(r.Verdict))
	if u := r.Uncertainty; u != nil {
		row("Uncertainty", fmt.Sprintf("mean %s, p5 %s, p50 %s, p95 %s  (%d samples, seed %d)", Percent(u.Mean), Percent(u.P5), Percent(u.P50), Percent(u.P95), u.Samples, u.Seed))
		row("P(meets target)", fmt.Sprintf("%.1f%%", u.PMeetsTarget*100))
	}
	row("Tier", tierText(r))

	if len(r.Levers) > 0 {
		b.WriteString("\nBiggest levers (gain if made perfect):\n")
		for i, l := range r.Levers {
			if i == topLevers {
				break
			}
			fmt.Fprintf(&b, "  %d. %-20s %s  +%.4f pp  (%s less downtime / %dd)\n", i+1, l.Component, Percent(l.Availability),
				l.ImprovementPotential*100, minutes(l.ImprovementPotential*float64(r.WindowDays)*24*60), r.WindowDays)
		}
	}
	if r.Method == eval.MethodBDD {
		if n := len(r.SPOFs); n > 0 {
			shown, more := r.SPOFs, ""
			if n > maxSPOFsShown {
				shown, more = r.SPOFs[:maxSPOFsShown], fmt.Sprintf(" and %d more", n-maxSPOFsShown)
			}
			fmt.Fprintf(&b, "\nSingle points of failure: %s%s\n", strings.Join(shown, ", "), more)
		} else {
			b.WriteString("\nSingle points of failure: none\n")
		}
		if len(r.CutSets) > 0 {
			parts := make([]string, len(r.CutSets))
			for i, p := range r.CutSets {
				parts[i] = strings.Join(p, " + ")
			}
			more := ""
			if r.CutSetsTruncated {
				more = ", ..."
			}
			fmt.Fprintf(&b, "Pairs that take it down together: %s%s\n", strings.Join(parts, "; "), more)
		}
	}
	for _, w := range r.Warnings {
		fmt.Fprintf(&b, "warning: %s\n", w)
	}
	fmt.Fprintf(&b, "\n%s\n", r.Recommendation)
	return b.String()
}

func checkMarkdown(r *eval.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", r.Service)
	fmt.Fprintf(&b, "**Verdict: %s.** %s\n\n", strings.ToUpper(r.Verdict), r.Recommendation)
	b.WriteString("| | Availability | Downtime per window |\n|---|---|---|\n")
	line := func(label string, a float64) {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", label, Percent(a), Downtime(a, r.WindowDays))
	}
	line("Target", r.Target)
	line("Computed", r.Availability)
	if showNaive(r) {
		line("Naive multiplication", *r.Naive)
	}
	if r.Ceiling != nil {
		line("Dependency ceiling", *r.Ceiling)
	}
	fmt.Fprintf(&b, "\nRecommended tier: **%s**. Window: %d days.\n", tierText(r), r.WindowDays)
	if r.AvailabilityCI != nil {
		fmt.Fprintf(&b, "\nMonte Carlo 95%% interval: %s to %s.\n", Percent(r.AvailabilityCI[0]), Percent(r.AvailabilityCI[1]))
	}
	if u := r.Uncertainty; u != nil {
		fmt.Fprintf(&b, "\n## Uncertainty\n\nMean %s, p5 %s, p50 %s, p95 %s over %d samples (seed %d). Probability of meeting the target: **%.1f%%**.\n",
			Percent(u.Mean), Percent(u.P5), Percent(u.P50), Percent(u.P95), u.Samples, u.Seed, u.PMeetsTarget*100)
	}
	if len(r.Levers) > 0 {
		b.WriteString("\n## Biggest levers\n\n| Component | Availability | Gain if made perfect |\n|---|---|---|\n")
		for i, l := range r.Levers {
			if i == topLevers {
				break
			}
			fmt.Fprintf(&b, "| %s | %s | +%.4f pp |\n", l.Component, Percent(l.Availability), l.ImprovementPotential*100)
		}
	}
	if r.Method == eval.MethodBDD {
		b.WriteString("\n## Single points of failure\n\n")
		if len(r.SPOFs) == 0 {
			b.WriteString("None.\n")
		}
		for _, s := range r.SPOFs {
			fmt.Fprintf(&b, "- %s\n", s)
		}
		if len(r.CutSets) > 0 {
			b.WriteString("\n## Pairs that take it down together\n\n")
			for _, p := range r.CutSets {
				fmt.Fprintf(&b, "- %s\n", strings.Join(p, " + "))
			}
		}
	}
	if len(r.Warnings) > 0 {
		b.WriteString("\n## Warnings\n\n")
		for _, w := range r.Warnings {
			fmt.Fprintf(&b, "- %s\n", w)
		}
	}
	return b.String()
}
