package analysis

import (
	"fmt"

	"github.com/marcelblanarik/nines/internal/eval"
	"github.com/marcelblanarik/nines/internal/model"
)

// Set replaces one component's availability (a percentage) in a what-if.
type Set struct {
	Component string  `json:"component"`
	Percent   float64 `json:"percent"`
}

// DiffRow compares one headline number before and after.
type DiffRow struct {
	Label  string  `json:"label"`
	Before float64 `json:"before"`
	After  float64 `json:"after"`
	// DeltaPP is the change in percentage points.
	DeltaPP float64 `json:"delta_pp"`
	// DeltaDowntimeMin is the change in allowed downtime per window, in minutes (negative is better).
	DeltaDowntimeMin float64 `json:"delta_downtime_min"`
}

// Diff is a what-if compared with the baseline.
type Diff struct {
	SchemaVersion int          `json:"schema_version"`
	Changes       []string     `json:"changes"`
	Rows          []DiffRow    `json:"rows"`
	Before        *eval.Result `json:"before"`
	After         *eval.Result `json:"after"`
}

// WhatIf applies removals, then availability overrides, to a copy of the model and compares the result
// with the baseline. The original model is not modified.
func WhatIf(m *model.Model, sets []Set, removes []string, opt Options) (*Diff, error) {
	if len(sets) == 0 && len(removes) == 0 {
		return nil, fmt.Errorf("nothing to change: pass at least one --set name=value or --remove name")
	}
	before, err := Check(m, opt)
	if err != nil {
		return nil, err
	}
	mod := m
	var changes []string
	for _, name := range removes {
		if mod, err = mod.Without(name); err != nil {
			return nil, err
		}
		changes = append(changes, "remove "+name)
	}
	for _, s := range sets {
		old := mod.Lookup(s.Component)
		if old == nil {
			return nil, fmt.Errorf("unknown component %q", s.Component)
		}
		if mod, err = mod.WithAvailability(s.Component, model.PointAvailability(s.Percent)); err != nil {
			return nil, err
		}
		changes = append(changes, fmt.Sprintf("set %s to %g%% (was %g%%)", s.Component, s.Percent, old.Avail.Value()*100))
	}
	after, err := Check(mod, opt)
	if err != nil {
		return nil, err
	}
	d := &Diff{SchemaVersion: eval.SchemaVersion, Changes: changes, Before: before, After: after}
	days := float64(m.WindowDays)
	row := func(label string, b, a float64) {
		d.Rows = append(d.Rows, DiffRow{
			Label: label, Before: b, After: a,
			DeltaPP:          (a - b) * 100,
			DeltaDowntimeMin: (b - a) * days * 24 * 60,
		})
	}
	row("computed", before.Availability, after.Availability)
	if before.Ceiling != nil && after.Ceiling != nil {
		row("ceiling", *before.Ceiling, *after.Ceiling)
	}
	return d, nil
}
