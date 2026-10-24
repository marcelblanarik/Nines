package model

import (
	"errors"
	"fmt"
	"strings"
)

// Preorder lists every node of the requires tree, each before its children. The position in this list is the
// node's index in the structural edits below and in the graph the web app draws.
func Preorder(root *Node) []*Node {
	var out []*Node
	var walk func(*Node)
	walk = func(n *Node) {
		out = append(out, n)
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
	return out
}

func cloneNode(n *Node) *Node {
	c := *n
	c.Children = make([]*Node, len(n.Children))
	for i, k := range n.Children {
		c.Children[i] = cloneNode(k)
	}
	return &c
}

// clone copies the parts an edit may change, so the original model is never modified.
func (m *Model) clone() *Model {
	out := *m
	out.Components = append([]Component(nil), m.Components...)
	out.Requires = cloneNode(m.Requires)
	return &out
}

// Revalidate renders m and parses it again, so an edited model gets every check a file gets (ranges, groups,
// cycles, k <= n) and fresh line numbers. The edits below call it before returning.
func Revalidate(m *Model) (*Model, error) {
	text, err := Format(m)
	if err != nil {
		return nil, err
	}
	return Parse(text)
}

// AddComponent appends a component. Nothing uses it until a requirement refers to it.
func (m *Model) AddComponent(c Component) (*Model, error) {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return nil, errors.New("a component needs a name")
	}
	if m.Lookup(c.Name) != nil {
		return nil, fmt.Errorf("component %q already exists", c.Name)
	}
	out := m.clone()
	out.Components = append(out.Components, c)
	return Revalidate(out)
}

// UpdateComponent replaces everything about a component except its name.
func (m *Model) UpdateComponent(name string, c Component) (*Model, error) {
	out := m.clone()
	for i := range out.Components {
		if out.Components[i].Name == name {
			c.Name, c.Line = name, out.Components[i].Line
			out.Components[i] = c
			return Revalidate(out)
		}
	}
	return nil, fmt.Errorf("unknown component %q", name)
}

func (m *Model) mustHave(name string) error {
	if m.Lookup(name) == nil {
		return fmt.Errorf("unknown component %q", name)
	}
	return nil
}

// AddRequirement makes the service need one more component, in series with everything else: it joins the
// top-level "all", or wraps the current requirement in one.
func (m *Model) AddRequirement(name string) (*Model, error) {
	if err := m.mustHave(name); err != nil {
		return nil, err
	}
	out := m.clone()
	ref := &Node{Kind: KindRef, Name: name}
	if out.Requires.Kind == KindAll {
		out.Requires.Children = append(out.Requires.Children, ref)
	} else {
		out.Requires = &Node{Kind: KindAll, Children: []*Node{out.Requires, ref}}
	}
	return Revalidate(out)
}

// AddRedundant makes name an alternative to the first place the model uses "to": that use becomes
// "any of it and name", or name joins the "any" that already holds it.
func (m *Model) AddRedundant(name, to string) (*Model, error) {
	if err := m.mustHave(name); err != nil {
		return nil, err
	}
	if err := m.mustHave(to); err != nil {
		return nil, err
	}
	out := m.clone()
	ref := &Node{Kind: KindRef, Name: name}
	uses := func(n *Node) bool { return (n.Kind == KindRef || n.Kind == KindRetry) && n.Name == to }
	var rewrite func(n *Node) (*Node, bool)
	rewrite = func(n *Node) (*Node, bool) {
		if uses(n) {
			return &Node{Kind: KindAny, Children: []*Node{n, ref}}, true
		}
		if n.Kind == KindAny {
			for _, c := range n.Children {
				if uses(c) {
					n.Children = append(n.Children, ref)
					return n, true
				}
			}
		}
		for i, c := range n.Children {
			if r, ok := rewrite(c); ok {
				n.Children[i] = r
				return n, true
			}
		}
		return n, false
	}
	root, ok := rewrite(out.Requires)
	if !ok {
		return nil, fmt.Errorf("%q is not used by the model, so there is nothing to make it redundant with", to)
	}
	out.Requires = root
	return Revalidate(out)
}

func nodeAt(m *Model, index int) (*Node, error) {
	nodes := Preorder(m.Requires)
	if index < 0 || index >= len(nodes) {
		return nil, fmt.Errorf("no node %d in the requires tree", index)
	}
	return nodes[index], nil
}

// AddChild adds a component to an all, any or k_of_n node.
func (m *Model) AddChild(index int, name string) (*Model, error) {
	if err := m.mustHave(name); err != nil {
		return nil, err
	}
	out := m.clone()
	n, err := nodeAt(out, index)
	if err != nil {
		return nil, err
	}
	switch n.Kind {
	case KindAll, KindAny, KindKOfN:
		n.Children = append(n.Children, &Node{Kind: KindRef, Name: name})
	default:
		return nil, errors.New("only all, any and k_of_n nodes take more members")
	}
	return Revalidate(out)
}

// NodeEdit changes one operator node; only the fields that are set apply.
type NodeEdit struct {
	// Kind switches an all to an any or back.
	Kind     *Kind
	K        *int
	Attempts *int
	Ratio    *float64
}

// SetNode edits an operator node's parameters.
func (m *Model) SetNode(index int, e NodeEdit) (*Model, error) {
	out := m.clone()
	n, err := nodeAt(out, index)
	if err != nil {
		return nil, err
	}
	if e.Kind != nil {
		if (n.Kind != KindAll && n.Kind != KindAny) || (*e.Kind != KindAll && *e.Kind != KindAny) {
			return nil, errors.New("only an all can become an any, and the other way round")
		}
		n.Kind = *e.Kind
	}
	if e.K != nil {
		if n.Kind != KindKOfN {
			return nil, errors.New("k only applies to k_of_n")
		}
		n.K = *e.K
	}
	if e.Attempts != nil {
		if n.Kind != KindRetry {
			return nil, errors.New("attempts only applies to retry")
		}
		n.Attempts = *e.Attempts
	}
	if e.Ratio != nil {
		if n.Kind != KindFallback && n.Kind != KindCached && n.Kind != KindOptional {
			return nil, errors.New("this node has no ratio")
		}
		n.Ratio = *e.Ratio
	}
	return Revalidate(out)
}
