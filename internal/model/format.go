package model

import (
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Format renders m as canonical YAML (comments from the source are not kept).
func Format(m *Model) ([]byte, error) {
	doc := mapping()
	add(doc, "service", str(m.Service))
	add(doc, "target", num(m.Target))
	add(doc, "window", str(strconv.Itoa(m.WindowDays)+"d"))
	comps := mapping()
	for _, c := range m.Components {
		cm := mapping()
		add(cm, "availability", availNode(c.Avail))
		if c.Owned {
			add(cm, "owned", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
		}
		if c.Source != "" {
			add(cm, "source", str(c.Source))
		}
		if c.TransientFraction != 0 {
			add(cm, "transient_fraction", num(c.TransientFraction))
		}
		if c.Group != "" {
			add(cm, "group", str(c.Group))
		}
		cm.Style = yaml.FlowStyle
		add(comps, c.Name, cm)
	}
	add(doc, "components", comps)
	add(doc, "requires", nodeYAML(m.Requires))
	return yaml.Marshal(doc)
}

func mapping() *yaml.Node { return &yaml.Node{Kind: yaml.MappingNode} }

func add(m *yaml.Node, key string, v *yaml.Node) {
	m.Content = append(m.Content, str(key), v)
}

func str(s string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s} }

// num renders a float so that it reads back as a float: "99" would be an integer, so it becomes "99.0".
func num(f float64) *yaml.Node {
	v := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(v, ".") {
		v += ".0"
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: v}
}

func integer(i int) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(i)}
}

func flowSeq(items ...*yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle, Content: items}
}

func availNode(a Availability) *yaml.Node {
	switch a.Kind {
	case Interval:
		m := mapping()
		add(m, "interval", flowSeq(num(a.Lo), num(a.Hi)))
		m.Style = yaml.FlowStyle
		return m
	case Counts:
		c := mapping()
		add(c, "good", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.FormatInt(a.Good, 10)})
		add(c, "total", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.FormatInt(a.Total, 10)})
		c.Style = yaml.FlowStyle
		m := mapping()
		add(m, "counts", c)
		m.Style = yaml.FlowStyle
		return m
	}
	return num(a.Percent)
}

func nodeYAML(n *Node) *yaml.Node {
	list := func(kind string) *yaml.Node {
		m := mapping()
		seq := &yaml.Node{Kind: yaml.SequenceNode}
		for _, c := range n.Children {
			seq.Content = append(seq.Content, nodeYAML(c))
		}
		add(m, kind, seq)
		return m
	}
	wrap := func(kind string, body *yaml.Node) *yaml.Node {
		m := mapping()
		add(m, kind, body)
		return m
	}
	switch n.Kind {
	case KindAll:
		return list("all")
	case KindAny:
		return list("any")
	case KindKOfN:
		body := mapping()
		add(body, "k", integer(n.K))
		of := &yaml.Node{Kind: yaml.SequenceNode}
		for _, c := range n.Children {
			of.Content = append(of.Content, nodeYAML(c))
		}
		add(body, "of", of)
		return wrap("k_of_n", body)
	case KindRetry:
		body := mapping()
		add(body, "of", str(n.Name))
		add(body, "attempts", integer(n.Attempts))
		body.Style = yaml.FlowStyle
		return wrap("retry", body)
	case KindFallback:
		body := mapping()
		add(body, "primary", nodeYAML(n.Children[0]))
		add(body, "fallback", nodeYAML(n.Children[1]))
		add(body, "coverage", num(n.Ratio))
		return wrap("fallback", body)
	case KindCached, KindOptional:
		kind, key := "cached", "hit_ratio"
		if n.Kind == KindOptional {
			kind, key = "optional", "touch"
		}
		body := mapping()
		add(body, "of", nodeYAML(n.Children[0]))
		add(body, key, num(n.Ratio))
		return wrap(kind, body)
	}
	return str(n.Name)
}
