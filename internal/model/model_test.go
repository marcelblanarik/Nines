package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const sample = `
service: checkout
target: 99.95
components:
  self:       {availability: 99.98, owned: true}
  auth:       {availability: "99.95%", source: measured}
  payments:   {availability: {interval: [99.8, 99.95]}, transient_fraction: 0.8}
  db-primary: {availability: {counts: {good: 999500, total: 1000000}}, group: az-a}
  db-replica: {availability: 99.95, group: az-b}
  az-a:       {availability: 99.99}
  az-b:       {availability: 99.99}
  spare:      {availability: 99}
requires:
  all:
    - self
    - auth
    - retry: {of: payments, attempts: 3}
    - any: [db-primary, db-replica]
`

func TestParseSample(t *testing.T) {
	m, err := Parse([]byte(sample))
	require.NoError(t, err)
	require.Equal(t, "checkout", m.Service)
	require.Equal(t, 99.95, m.Target)
	require.Equal(t, 30, m.WindowDays)
	require.Len(t, m.Components, 8)
	require.Equal(t, 99.95, m.Lookup("auth").Avail.Percent)
	require.Equal(t, Interval, m.Lookup("payments").Avail.Kind)
	require.InDelta(t, 0.9995, m.Lookup("db-primary").Avail.Value(), 1e-12)
	require.Equal(t, KindAll, m.Requires.Kind)
	require.Len(t, m.Requires.Children, 4)
	require.Equal(t, 3, m.Requires.Children[2].Attempts)
	require.Len(t, m.Warnings, 1)
	require.Contains(t, m.Warnings[0].Msg, "spare")
}

func TestErrorsCarryLineNumbers(t *testing.T) {
	cases := []struct {
		name, yaml, want string
		line             int
	}{
		{"fraction", "service: s\ntarget: 99\ncomponents:\n  a: {availability: 0.9995}\nrequires: a\n", "did you mean 99.95?", 4},
		{"low", "service: s\ntarget: 99\ncomponents:\n  a: {availability: 40}\nrequires: a\n", "outside the allowed range", 4},
		{"unknown ref", "service: s\ntarget: 99\ncomponents:\n  a: {availability: 99}\nrequires:\n  all:\n    - a\n    - zzz\n", `unknown component "zzz"`, 8},
		{"k too big", "service: s\ntarget: 99\ncomponents:\n  a: {availability: 99}\nrequires:\n  k_of_n: {k: 2, of: [a]}\n", "1 <= k <= n", 6},
		{"attempts", "service: s\ntarget: 99\ncomponents:\n  a: {availability: 99}\nrequires:\n  retry: {of: a, attempts: 0}\n", "attempts", 6},
		{"ratio", "service: s\ntarget: 99\ncomponents:\n  a: {availability: 99}\nrequires:\n  optional: {of: a, touch: 2}\n", "between 0 and 1", 6},
		{"group cycle", "service: s\ntarget: 99\ncomponents:\n  a: {availability: 99, group: b}\n  b: {availability: 99, group: a}\nrequires: a\n", "cycle", 4},
		{"unknown group", "service: s\ntarget: 99\ncomponents:\n  a: {availability: 99, group: q}\nrequires: a\n", `group "q"`, 4},
		{"duplicate", "service: s\ntarget: 99\ncomponents:\n  a: {availability: 99}\n  a: {availability: 98}\nrequires: a\n", "duplicate", 5},
		{"alias", "service: s\ntarget: 99\ncomponents:\n  a: &x {availability: 99}\n  b: *x\nrequires: a\n", "aliases", 5},
		{"bad yaml", "service: [", "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse([]byte(c.yaml))
			require.Error(t, err)
			var errs ErrorList
			require.ErrorAs(t, err, &errs)
			found := false
			for _, e := range errs {
				if strings.Contains(e.Msg, c.want) && (c.line == 0 || e.Line == c.line) {
					found = true
				}
			}
			require.True(t, found, "want %q at line %d, got %v", c.want, c.line, err)
		})
	}
}

func TestFormatRoundTrip(t *testing.T) {
	m, err := Parse([]byte(sample))
	require.NoError(t, err)
	out, err := Format(m)
	require.NoError(t, err)
	m2, err := Parse(out)
	require.NoError(t, err, string(out))
	out2, err := Format(m2)
	require.NoError(t, err)
	require.Equal(t, string(out), string(out2))
	require.Equal(t, m.Components[2].Avail, m2.Components[2].Avail)
}

func TestWithout(t *testing.T) {
	m, err := Parse([]byte(sample))
	require.NoError(t, err)
	r, err := m.Without("db-replica")
	require.NoError(t, err)
	require.Equal(t, KindAny, r.Requires.Children[3].Kind)
	require.Len(t, r.Requires.Children[3].Children, 1)
	r, err = m.Without("auth")
	require.NoError(t, err)
	require.Len(t, r.Requires.Children, 3)
	_, err = m.Without("nope")
	require.Error(t, err)
	// original is untouched
	require.Len(t, m.Requires.Children, 4)
}

func FuzzParse(f *testing.F) {
	f.Add([]byte(sample))
	f.Add([]byte("service: s\ntarget: 99\ncomponents: {a: {availability: 99}}\nrequires: {fallback: {primary: a, fallback: a, coverage: 0.5}}\n"))
	f.Add([]byte("a: &x [*x]"))
	if seeds, err := filepath.Glob("../../testdata/models/*.yaml"); err == nil {
		for _, s := range seeds {
			if b, err := os.ReadFile(s); err == nil {
				f.Add(b)
			}
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := Parse(data)
		if err == nil {
			_, _ = Format(m)
		}
	})
}
