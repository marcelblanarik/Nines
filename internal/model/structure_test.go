package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const base = `
service: s
target: 99.9
components:
  a: {availability: 99.9}
  b: {availability: 99.9}
  c: {availability: 99.5}
requires:
  all:
    - a
    - retry: {of: b, attempts: 2}
`

func mustParse(t *testing.T, src string) *Model {
	t.Helper()
	m, err := Parse([]byte(src))
	require.NoError(t, err)
	return m
}

func TestAddComponentAndRequirement(t *testing.T) {
	m := mustParse(t, base)
	m2, err := m.AddComponent(Component{Name: " d ", Avail: PointAvailability(99.99), Owned: true})
	require.NoError(t, err)
	require.NotNil(t, m2.Lookup("d"))
	require.True(t, m2.Lookup("d").Owned)
	require.Nil(t, m.Lookup("d"), "original is untouched")

	m3, err := m2.AddRequirement("d")
	require.NoError(t, err)
	require.Len(t, m3.Requires.Children, 3)
	require.Equal(t, "d", m3.Requires.Children[2].Name)

	_, err = m.AddComponent(Component{Name: "a", Avail: PointAvailability(99)})
	require.ErrorContains(t, err, "already exists")
	_, err = m.AddComponent(Component{Name: "", Avail: PointAvailability(99)})
	require.Error(t, err)
	_, err = m.AddComponent(Component{Name: "x", Avail: PointAvailability(0.5)})
	require.ErrorContains(t, err, "fraction")
}

func TestAddRequirementWrapsANonAllRoot(t *testing.T) {
	m := mustParse(t, "service: s\ntarget: 99\ncomponents:\n  a: {availability: 99}\n  b: {availability: 99}\nrequires: a\n")
	m2, err := m.AddRequirement("b")
	require.NoError(t, err)
	require.Equal(t, KindAll, m2.Requires.Kind)
	require.Len(t, m2.Requires.Children, 2)
}

func TestAddRedundant(t *testing.T) {
	m := mustParse(t, base)
	m2, err := m.AddRedundant("c", "a")
	require.NoError(t, err)
	require.Equal(t, KindAny, m2.Requires.Children[0].Kind)
	require.Len(t, m2.Requires.Children[0].Children, 2)

	// redundant with something already in an any joins that any instead of nesting
	m3, err := m2.AddComponent(Component{Name: "e", Avail: PointAvailability(99)})
	require.NoError(t, err)
	m3, err = m3.AddRedundant("e", "a")
	require.NoError(t, err)
	require.Len(t, m3.Requires.Children[0].Children, 3)

	// redundant with a retried component wraps the retry
	m4, err := m.AddRedundant("c", "b")
	require.NoError(t, err)
	require.Equal(t, KindAny, m4.Requires.Children[1].Kind)
	require.Equal(t, KindRetry, m4.Requires.Children[1].Children[0].Kind)

	_, err = m.AddRedundant("c", "zzz")
	require.Error(t, err)
	unused := mustParse(t, "service: s\ntarget: 99\ncomponents:\n  a: {availability: 99}\n  u: {availability: 99}\n  v: {availability: 99}\nrequires: a\n")
	_, err = unused.AddRedundant("v", "u")
	require.ErrorContains(t, err, "not used")
}

func TestNodeIndexesAndEdits(t *testing.T) {
	m := mustParse(t, base)
	nodes := Preorder(m.Requires)
	require.Len(t, nodes, 3) // all, a, retry
	require.Equal(t, KindRetry, nodes[2].Kind)

	attempts := 5
	m2, err := m.SetNode(2, NodeEdit{Attempts: &attempts})
	require.NoError(t, err)
	require.Equal(t, 5, m2.Requires.Children[1].Attempts)
	require.Equal(t, 2, m.Requires.Children[1].Attempts, "original is untouched")

	asAny := KindAny
	m3, err := m.SetNode(0, NodeEdit{Kind: &asAny})
	require.NoError(t, err)
	require.Equal(t, KindAny, m3.Requires.Kind)

	bad := 0
	_, err = m.SetNode(2, NodeEdit{Attempts: &bad})
	require.Error(t, err, "revalidation rejects attempts < 1")
	_, err = m.SetNode(1, NodeEdit{Attempts: &attempts})
	require.Error(t, err, "a plain reference has no attempts")
	_, err = m.SetNode(9, NodeEdit{})
	require.Error(t, err)

	m4, err := m.AddChild(0, "c")
	require.NoError(t, err)
	require.Len(t, m4.Requires.Children, 3)
	_, err = m.AddChild(2, "c")
	require.Error(t, err)
}

// Whole-number floats must print as 99.0, not as an explicit !!float tag.
func TestFormatWholeNumberFloats(t *testing.T) {
	m := mustParse(t, "service: s\ntarget: 99\ncomponents:\n  a: {availability: {interval: [99, 100]}, transient_fraction: 1}\nrequires: a\n")
	out, err := Format(m)
	require.NoError(t, err)
	require.NotContains(t, string(out), "!!")
	require.Contains(t, string(out), "interval: [99.0, 100.0]")
	_, err = Parse(out)
	require.NoError(t, err)
}

func TestUpdateComponent(t *testing.T) {
	m := mustParse(t, base)
	m2, err := m.UpdateComponent("a", Component{
		Avail: Availability{Kind: Interval, Lo: 99.5, Hi: 99.95}, Owned: true, Group: "c", TransientFraction: 0.5, Source: "vendor-sla",
	})
	require.NoError(t, err)
	a := m2.Lookup("a")
	require.Equal(t, Interval, a.Avail.Kind)
	require.Equal(t, "c", a.Group)
	require.Equal(t, "a", a.Name)

	_, err = m.UpdateComponent("a", Component{Avail: PointAvailability(99), Group: "a"})
	require.ErrorContains(t, err, "cycle")
	_, err = m.UpdateComponent("a", Component{Avail: PointAvailability(99), Group: "nope"})
	require.Error(t, err)
	_, err = m.UpdateComponent("nope", Component{Avail: PointAvailability(99)})
	require.Error(t, err)
}
