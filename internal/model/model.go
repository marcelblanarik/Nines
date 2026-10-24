// Package model holds the parsed, validated service description.
package model

import (
	"fmt"
	"sort"
	"strings"
)

// Kind is the type of a node in the requires tree.
type Kind int

const (
	KindRef Kind = iota
	KindAll
	KindAny
	KindKOfN
	KindRetry
	KindFallback
	KindCached
	KindOptional
)

// Node is one node of the requires tree.
type Node struct {
	Kind Kind
	Line int
	// Name is the component for KindRef and KindRetry.
	Name string
	// Children: members for all/any/k_of_n; [primary, fallback] for fallback; [of] for cached/optional.
	Children []*Node
	K        int
	Attempts int
	// Ratio is coverage (fallback), hit_ratio (cached) or touch (optional).
	Ratio float64
}

// AvailKind selects how a component's availability is described.
type AvailKind int

const (
	Point AvailKind = iota
	Interval
	Counts
)

// Availability is stored as written in the file: percentages, or raw counts.
type Availability struct {
	Kind        AvailKind
	Percent     float64 // Point
	Lo, Hi      float64 // Interval, percent
	Good, Total int64   // Counts
}

// Value is the point estimate as a fraction: the midpoint of an interval, good/total for counts.
func (a Availability) Value() float64 {
	switch a.Kind {
	case Interval:
		return (a.Lo + a.Hi) / 200
	case Counts:
		return float64(a.Good) / float64(a.Total)
	}
	return a.Percent / 100
}

// PointAvailability is a plain percentage.
func PointAvailability(percent float64) Availability {
	return Availability{Kind: Point, Percent: percent}
}

// Component is one entry of the components section.
type Component struct {
	Name              string
	Line              int
	Avail             Availability
	Owned             bool
	Source            string
	TransientFraction float64
	Group             string
}

// Warning is a non-fatal finding.
type Warning struct {
	Line int
	Msg  string
}

func (w Warning) String() string {
	if w.Line > 0 {
		return fmt.Sprintf("line %d: %s", w.Line, w.Msg)
	}
	return w.Msg
}

// Model is a validated service description.
type Model struct {
	Service    string
	Target     float64 // percent
	WindowDays int
	Components []Component // declaration order
	Requires   *Node
	Warnings   []Warning
}

// DefaultWindowDays is used when the file has no window.
const DefaultWindowDays = 30

// Lookup returns the named component or nil.
func (m *Model) Lookup(name string) *Component {
	for i := range m.Components {
		if m.Components[i].Name == name {
			return &m.Components[i]
		}
	}
	return nil
}

// Error is a validation error with the 1-based line it refers to (0 if unknown).
type Error struct {
	Line int
	Msg  string
}

func (e Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
	}
	return e.Msg
}

// ErrorList is returned by Parse for an invalid model.
type ErrorList []Error

func (l ErrorList) Error() string {
	parts := make([]string, len(l))
	for i, e := range l {
		parts[i] = e.Error()
	}
	return strings.Join(parts, "\n")
}

func (l ErrorList) sorted() ErrorList {
	sort.SliceStable(l, func(i, j int) bool { return l[i].Line < l[j].Line })
	return l
}
