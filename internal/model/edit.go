package model

import "fmt"

// WithAvailability returns a copy of m with one component's availability replaced.
func (m *Model) WithAvailability(name string, a Availability) (*Model, error) {
	out := *m
	out.Components = append([]Component(nil), m.Components...)
	for i := range out.Components {
		if out.Components[i].Name == name {
			out.Components[i].Avail = a
			return &out, nil
		}
	}
	return nil, fmt.Errorf("unknown component %q", name)
}

// Without returns a copy of m with a component deleted. Every reference to it disappears from the
// requires tree: all/any/k_of_n lose that member (n shrinks, k stays); a node left with no members, a
// retry/cached/optional of it, or a fallback whose primary it was, disappears too; a fallback that loses
// only its fallback becomes its primary. Components that had it as their group lose the group.
func (m *Model) Without(name string) (*Model, error) {
	if m.Lookup(name) == nil {
		return nil, fmt.Errorf("unknown component %q", name)
	}
	out := *m
	out.Components = nil
	for _, c := range m.Components {
		if c.Name == name {
			continue
		}
		if c.Group == name {
			c.Group = ""
		}
		out.Components = append(out.Components, c)
	}
	req, err := prune(m.Requires, name)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("removing %q leaves nothing to evaluate", name)
	}
	out.Requires = req
	out.Warnings = warnings(&out)
	return &out, nil
}

func prune(n *Node, name string) (*Node, error) {
	switch n.Kind {
	case KindRef, KindRetry:
		if n.Name == name {
			return nil, nil
		}
		return n, nil
	}
	var kids []*Node
	var gone []bool
	for _, c := range n.Children {
		pc, err := prune(c, name)
		if err != nil {
			return nil, err
		}
		gone = append(gone, pc == nil)
		if pc != nil {
			kids = append(kids, pc)
		}
	}
	out := *n
	out.Children = kids
	switch n.Kind {
	case KindAll, KindAny:
		if len(kids) == 0 {
			return nil, nil
		}
	case KindKOfN:
		if len(kids) == 0 {
			return nil, nil
		}
		if n.K > len(kids) {
			return nil, fmt.Errorf("removing %q leaves k_of_n at line %d with k=%d but only %d members", name, n.Line, n.K, len(kids))
		}
	case KindFallback:
		switch {
		case gone[0]:
			return nil, nil
		case gone[1]:
			return kids[0], nil
		}
	case KindCached, KindOptional:
		if gone[0] {
			return nil, nil
		}
	}
	return &out, nil
}
