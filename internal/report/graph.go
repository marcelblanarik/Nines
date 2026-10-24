package report

import (
	"fmt"
	"strings"

	"github.com/marcelblanarik/nines/internal/model"
)

// GraphNode is one box in the dependency graph. Kind is "service", "component" or an operator name.
type GraphNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
	Owned bool   `json:"owned,omitempty"`
	// Component and Availability (a percentage) are set for components only.
	Component    string  `json:"component,omitempty"`
	Availability float64 `json:"availability,omitempty"`
	// Info is everything the editor needs about a component.
	Info *ComponentInfo `json:"info,omitempty"`
	// Node is an operator's index in model.Preorder, which the structural edits address it by.
	Node   *int        `json:"node,omitempty"`
	Params *NodeParams `json:"params,omitempty"`
}

// ComponentInfo describes a component as written in the model: Kind is "point", "interval" or "counts".
type ComponentInfo struct {
	Kind              string  `json:"kind"`
	Percent           float64 `json:"percent"`
	Lo                float64 `json:"lo"`
	Hi                float64 `json:"hi"`
	Good              int64   `json:"good"`
	Total             int64   `json:"total"`
	Owned             bool    `json:"owned"`
	Source            string  `json:"source"`
	TransientFraction float64 `json:"transient_fraction"`
	Group             string  `json:"group"`
}

// NodeParams are an operator's tunable parameters; fields that do not apply to its kind are zero.
type NodeParams struct {
	K        int     `json:"k,omitempty"`
	Attempts int     `json:"attempts,omitempty"`
	Ratio    float64 `json:"ratio,omitempty"`
	Members  int     `json:"members"`
}

func componentInfo(c *model.Component) *ComponentInfo {
	info := &ComponentInfo{Owned: c.Owned, Source: c.Source, TransientFraction: c.TransientFraction, Group: c.Group}
	switch c.Avail.Kind {
	case model.Interval:
		info.Kind, info.Lo, info.Hi = "interval", c.Avail.Lo, c.Avail.Hi
		info.Percent = c.Avail.Value() * 100
	case model.Counts:
		info.Kind, info.Good, info.Total = "counts", c.Avail.Good, c.Avail.Total
		info.Percent = c.Avail.Value() * 100
	default:
		info.Kind, info.Percent = "point", c.Avail.Percent
	}
	return info
}

// GraphEdge points from a dependent to the thing it needs. Kind is "requires" or "group".
type GraphEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label,omitempty"`
	Kind  string `json:"kind"`
}

// GraphData is the model as nodes and edges. A component used in several places is one shared node.
type GraphData struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// AvailabilityLabel renders a component's availability for a graph box.
func AvailabilityLabel(a model.Availability) string {
	switch a.Kind {
	case model.Interval:
		return fmt.Sprintf("%g-%g%%", a.Lo, a.Hi)
	case model.Counts:
		return Percent(a.Value())
	}
	return fmt.Sprintf("%g%%", a.Percent)
}

// Graph converts a model to nodes and edges.
func Graph(m *model.Model) *GraphData {
	g := &GraphData{}
	seen := map[string]bool{}
	ops := 0

	var component func(name string) string
	component = func(name string) string {
		id := "c_" + sanitize(name)
		if seen[id] {
			return id
		}
		seen[id] = true
		c := m.Lookup(name)
		g.Nodes = append(g.Nodes, GraphNode{
			ID: id, Kind: "component", Component: name, Owned: c.Owned, Info: componentInfo(c),
			Availability: c.Avail.Value() * 100,
			Label:        name + "\n" + AvailabilityLabel(c.Avail),
		})
		if c.Group != "" {
			g.Edges = append(g.Edges, GraphEdge{From: id, To: component(c.Group), Label: "in group", Kind: "group"})
		}
		return id
	}
	index := map[*model.Node]int{}
	for i, n := range model.Preorder(m.Requires) {
		index[n] = i
	}
	op := func(n *model.Node, kind, label string) string {
		ops++
		id := fmt.Sprintf("op%d", ops)
		at := index[n]
		g.Nodes = append(g.Nodes, GraphNode{
			ID: id, Kind: kind, Label: label, Node: &at,
			Params: &NodeParams{K: n.K, Attempts: n.Attempts, Ratio: n.Ratio, Members: len(n.Children)},
		})
		return id
	}
	var walk func(n *model.Node) string
	walk = func(n *model.Node) string {
		var id string
		var labels []string // per-child edge labels
		switch n.Kind {
		case model.KindRef:
			return component(n.Name)
		case model.KindRetry:
			id = op(n, "retry", fmt.Sprintf("retry up to %d times", n.Attempts))
			g.Edges = append(g.Edges, GraphEdge{From: id, To: component(n.Name), Kind: "requires"})
			return id
		case model.KindAll:
			id = op(n, "all", "all of")
		case model.KindAny:
			id = op(n, "any", "any of")
		case model.KindKOfN:
			id = op(n, "k_of_n", fmt.Sprintf("%d of %d", n.K, len(n.Children)))
		case model.KindFallback:
			id = op(n, "fallback", fmt.Sprintf("fallback, covers %g%%", n.Ratio*100))
			labels = []string{"primary", "fallback"}
		case model.KindCached:
			id = op(n, "cached", fmt.Sprintf("cache, hit ratio %g%%", n.Ratio*100))
			labels = []string{"origin"}
		case model.KindOptional:
			id = op(n, "optional", fmt.Sprintf("optional, used by %g%%", n.Ratio*100))
		}
		for i, c := range n.Children {
			e := GraphEdge{From: id, To: walk(c), Kind: "requires"}
			if i < len(labels) {
				e.Label = labels[i]
			}
			g.Edges = append(g.Edges, e)
		}
		return id
	}

	g.Nodes = append(g.Nodes, GraphNode{ID: "svc", Kind: "service", Label: m.Service})
	g.Edges = append(g.Edges, GraphEdge{From: "svc", To: walk(m.Requires), Kind: "requires"})
	// Components nothing uses yet are still drawn, so they can be found, edited and connected.
	for _, c := range m.Components {
		component(c.Name)
	}
	return g
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else {
			fmt.Fprintf(&b, "_%x_", r)
		}
	}
	return b.String()
}

func mermaidText(s string) string {
	s = strings.NewReplacer(`"`, "#quot;", "<", "#lt;", ">", "#gt;").Replace(s)
	return strings.ReplaceAll(s, "\n", "<br/>")
}

// Mermaid renders the graph as a Mermaid flowchart annotated with availabilities.
func Mermaid(m *model.Model) string {
	g := Graph(m)
	var b strings.Builder
	b.WriteString("flowchart TD\n")
	for _, n := range g.Nodes {
		label := mermaidText(n.Label)
		switch n.Kind {
		case "service":
			fmt.Fprintf(&b, "  %s([\"%s\"])\n", n.ID, label)
		case "component":
			fmt.Fprintf(&b, "  %s[\"%s\"]\n", n.ID, label)
		default:
			fmt.Fprintf(&b, "  %s{{\"%s\"}}\n", n.ID, label)
		}
	}
	for _, e := range g.Edges {
		arrow := "-->"
		if e.Kind == "group" {
			arrow = "-.->"
		}
		if e.Label != "" {
			fmt.Fprintf(&b, "  %s %s|%s| %s\n", e.From, arrow, mermaidText(e.Label), e.To)
		} else {
			fmt.Fprintf(&b, "  %s %s %s\n", e.From, arrow, e.To)
		}
	}
	var owned []string
	for _, n := range g.Nodes {
		if n.Owned {
			owned = append(owned, n.ID)
		}
	}
	if len(owned) > 0 {
		b.WriteString("  classDef owned stroke-width:3px\n")
		fmt.Fprintf(&b, "  class %s owned\n", strings.Join(owned, ","))
	}
	return b.String()
}
