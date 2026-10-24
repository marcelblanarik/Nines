package model

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	maxDepth    = 64
	maxNodes    = 20000
	maxAttempts = 1000
	maxWindow   = 3650
)

var (
	windowRe   = regexp.MustCompile(`^(\d+)d$`)
	yamlLineRe = regexp.MustCompile(`line (\d+)`)
)

type parser struct {
	errs  ErrorList
	nodes int
}

func (p *parser) fail(line int, format string, args ...any) {
	p.errs = append(p.errs, Error{Line: line, Msg: fmt.Sprintf(format, args...)})
}

type kv struct {
	key       string
	keyNode   *yaml.Node
	valueNode *yaml.Node
}

// entries flattens a mapping node, rejecting duplicate and non-scalar keys.
func (p *parser) entries(n *yaml.Node, what string) []kv {
	if n.Kind == yaml.AliasNode {
		p.fail(n.Line, "YAML aliases are not supported (%s)", what)
		return nil
	}
	if n.Kind != yaml.MappingNode {
		p.fail(n.Line, "%s must be a mapping", what)
		return nil
	}
	seen := map[string]bool{}
	var out []kv
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if k.Kind != yaml.ScalarNode {
			p.fail(k.Line, "%s keys must be plain strings", what)
			continue
		}
		if seen[k.Value] {
			p.fail(k.Line, "duplicate key %q in %s", k.Value, what)
			continue
		}
		seen[k.Value] = true
		out = append(out, kv{k.Value, k, v})
	}
	return out
}

// Parse reads and validates a model. An invalid model yields an ErrorList.
func Parse(data []byte) (*Model, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		line := 0
		if m := yamlLineRe.FindStringSubmatch(err.Error()); m != nil {
			line, _ = strconv.Atoi(m[1])
		}
		return nil, ErrorList{{Line: line, Msg: strings.TrimPrefix(err.Error(), "yaml: ")}}
	}
	if len(doc.Content) == 0 {
		return nil, ErrorList{{Msg: "empty model"}}
	}
	p := &parser{}
	m := p.model(doc.Content[0])
	if len(p.errs) > 0 {
		return nil, p.errs.sorted()
	}
	m.Warnings = warnings(m)
	return m, nil
}

func (p *parser) model(root *yaml.Node) *Model {
	m := &Model{WindowDays: DefaultWindowDays}
	var comps, requires *yaml.Node
	hasTarget := false
	for _, e := range p.entries(root, "model") {
		switch e.key {
		case "service":
			m.Service = p.str(e.valueNode, "service")
			if m.Service == "" {
				p.fail(e.keyNode.Line, "service must not be empty")
			}
		case "target":
			hasTarget = true
			m.Target = p.percent(e.valueNode, "target")
			if m.Target >= 100 {
				p.fail(e.valueNode.Line, "target must be below 100 (a 100%% target leaves no error budget)")
			}
		case "window":
			m.WindowDays = p.window(e.valueNode)
		case "components":
			comps = e.valueNode
		case "requires":
			requires = e.valueNode
		default:
			p.fail(e.keyNode.Line, "unknown key %q", e.key)
		}
	}
	if m.Service == "" && len(p.errs) == 0 {
		p.fail(root.Line, "missing service")
	}
	if !hasTarget {
		p.fail(root.Line, "missing target")
	}
	if comps == nil {
		p.fail(root.Line, "missing components")
	} else {
		m.Components = p.components(comps)
	}
	if requires == nil {
		p.fail(root.Line, "missing requires")
	} else {
		names := map[string]bool{}
		for _, c := range m.Components {
			names[c.Name] = true
		}
		m.Requires = p.node(requires, names, 0)
	}
	p.checkGroups(m)
	return m
}

func (p *parser) scalar(n *yaml.Node, what string) (string, bool) {
	if n.Kind != yaml.ScalarNode {
		if n.Kind == yaml.AliasNode {
			p.fail(n.Line, "YAML aliases are not supported (%s)", what)
		} else {
			p.fail(n.Line, "%s must be a single value", what)
		}
		return "", false
	}
	return n.Value, true
}

func (p *parser) str(n *yaml.Node, what string) string {
	s, _ := p.scalar(n, what)
	return s
}

func (p *parser) boolean(n *yaml.Node, what string) bool {
	s, ok := p.scalar(n, what)
	if !ok {
		return false
	}
	b, err := strconv.ParseBool(s)
	if err != nil || n.Tag != "!!bool" {
		p.fail(n.Line, "%s must be true or false", what)
	}
	return b
}

func (p *parser) integer(n *yaml.Node, what string) int {
	s, ok := p.scalar(n, what)
	if !ok {
		return 0
	}
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		p.fail(n.Line, "%s must be an integer", what)
	}
	return int(v)
}

func (p *parser) ratio(n *yaml.Node, what string) float64 {
	s, ok := p.scalar(n, what)
	if !ok {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || v < 0 || v > 1 {
		p.fail(n.Line, "%s must be a number between 0 and 1", what)
		return 0
	}
	return v
}

func (p *parser) window(n *yaml.Node) int {
	s := p.str(n, "window")
	mt := windowRe.FindStringSubmatch(s)
	if mt == nil {
		p.fail(n.Line, "window must look like 30d")
		return DefaultWindowDays
	}
	d, err := strconv.Atoi(mt[1])
	if err != nil || d < 1 || d > maxWindow {
		p.fail(n.Line, "window must be between 1d and %dd", maxWindow)
		return DefaultWindowDays
	}
	return d
}

// ParsePercent parses "99.95" or "99.95%" and enforces the (50, 100] range.
func ParsePercent(s string) (float64, error) {
	v, err := percentValue(s)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func percentValue(s string) (float64, error) {
	t := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%"))
	v, err := strconv.ParseFloat(t, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("%q is not a percentage", s)
	}
	if v > 0 && v <= 1 {
		return 0, fmt.Errorf("%s looks like a fraction, not a percentage: did you mean %s?", s, strconv.FormatFloat(v*100, 'g', 12, 64))
	}
	if v <= 50 || v > 100 {
		return 0, fmt.Errorf("%s is outside the allowed range (50, 100]", s)
	}
	return v, nil
}

func (p *parser) percent(n *yaml.Node, what string) float64 {
	s, ok := p.scalar(n, what)
	if !ok {
		return 0
	}
	v, err := percentValue(s)
	if err != nil {
		p.fail(n.Line, "%s: %v", what, err)
		return 0
	}
	return v
}

func (p *parser) availability(n *yaml.Node) Availability {
	if n.Kind == yaml.ScalarNode {
		return PointAvailability(p.percent(n, "availability"))
	}
	es := p.entries(n, "availability")
	if len(es) != 1 {
		p.fail(n.Line, "availability must be a percentage, {interval: [lo, hi]} or {counts: {good, total}}")
		return Availability{}
	}
	e := es[0]
	switch e.key {
	case "interval":
		v := e.valueNode
		if v.Kind != yaml.SequenceNode || len(v.Content) != 2 {
			p.fail(v.Line, "interval must be [lo, hi]")
			return Availability{}
		}
		lo := p.percent(v.Content[0], "interval lower bound")
		hi := p.percent(v.Content[1], "interval upper bound")
		if lo > hi {
			p.fail(v.Line, "interval lower bound %g is above upper bound %g", lo, hi)
		}
		return Availability{Kind: Interval, Lo: lo, Hi: hi}
	case "counts":
		var good, total int64
		var hasGood, hasTotal bool
		for _, c := range p.entries(e.valueNode, "counts") {
			s := p.str(c.valueNode, c.key)
			x, err := strconv.ParseInt(s, 10, 64)
			if err != nil || x < 0 {
				p.fail(c.valueNode.Line, "counts.%s must be a non-negative integer", c.key)
			}
			switch c.key {
			case "good":
				good, hasGood = x, true
			case "total":
				total, hasTotal = x, true
			default:
				p.fail(c.keyNode.Line, "unknown key %q in counts", c.key)
			}
		}
		if !hasGood || !hasTotal || total <= 0 || good > total {
			p.fail(e.valueNode.Line, "counts needs good and total with 0 <= good <= total and total > 0")
		}
		return Availability{Kind: Counts, Good: good, Total: total}
	}
	p.fail(e.keyNode.Line, "availability form %q is not recognised (use interval or counts)", e.key)
	return Availability{}
}

func (p *parser) components(n *yaml.Node) []Component {
	var out []Component
	seen := map[string]bool{}
	if n.Kind != yaml.MappingNode {
		p.fail(n.Line, "components must be a mapping")
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		name := p.str(k, "component name")
		if name == "" {
			p.fail(k.Line, "component name must not be empty")
			continue
		}
		if seen[name] {
			p.fail(k.Line, "duplicate component %q", name)
			continue
		}
		seen[name] = true
		c := Component{Name: name, Line: k.Line}
		hasAvail := false
		for _, e := range p.entries(v, "component "+name) {
			switch e.key {
			case "availability":
				c.Avail, hasAvail = p.availability(e.valueNode), true
			case "owned":
				c.Owned = p.boolean(e.valueNode, "owned")
			case "source":
				c.Source = p.str(e.valueNode, "source")
			case "transient_fraction":
				c.TransientFraction = p.ratio(e.valueNode, "transient_fraction")
			case "group":
				c.Group = p.str(e.valueNode, "group")
			default:
				p.fail(e.keyNode.Line, "unknown key %q in component %q", e.key, name)
			}
		}
		if !hasAvail {
			p.fail(k.Line, "component %q has no availability", name)
		}
		out = append(out, c)
	}
	return out
}

var operators = map[string]bool{"all": true, "any": true, "k_of_n": true, "retry": true, "fallback": true, "cached": true, "optional": true}

func (p *parser) node(n *yaml.Node, names map[string]bool, depth int) *Node {
	p.nodes++
	if depth > maxDepth || p.nodes > maxNodes {
		if depth > maxDepth {
			p.fail(n.Line, "requires is nested deeper than %d levels", maxDepth)
		} else if p.nodes == maxNodes+1 {
			p.fail(n.Line, "requires has more than %d nodes", maxNodes)
		}
		return &Node{Kind: KindAll, Line: n.Line}
	}
	if n.Kind == yaml.ScalarNode {
		return p.ref(n, names)
	}
	es := p.entries(n, "requires node")
	if len(es) != 1 || !operators[es[0].key] {
		p.fail(n.Line, "a requires node is a component name or exactly one of: all, any, k_of_n, retry, fallback, cached, optional")
		return &Node{Kind: KindAll, Line: n.Line}
	}
	e := es[0]
	out := &Node{Line: e.keyNode.Line}
	switch e.key {
	case "all", "any":
		out.Kind = map[string]Kind{"all": KindAll, "any": KindAny}[e.key]
		out.Children = p.list(e.valueNode, e.key, names, depth)
	case "k_of_n":
		out.Kind = KindKOfN
		out.K = 0
		for _, f := range p.entries(e.valueNode, "k_of_n") {
			switch f.key {
			case "k":
				out.K = p.integer(f.valueNode, "k")
			case "of":
				out.Children = p.list(f.valueNode, "k_of_n.of", names, depth)
			default:
				p.fail(f.keyNode.Line, "unknown key %q in k_of_n", f.key)
			}
		}
		if out.K < 1 || out.K > len(out.Children) {
			p.fail(e.keyNode.Line, "k_of_n needs 1 <= k <= n (k=%d, n=%d)", out.K, len(out.Children))
		}
	case "retry":
		out.Kind = KindRetry
		for _, f := range p.entries(e.valueNode, "retry") {
			switch f.key {
			case "of":
				if f.valueNode.Kind != yaml.ScalarNode {
					p.fail(f.valueNode.Line, "retry.of must be a component name")
					break
				}
				out.Name = p.ref(f.valueNode, names).Name
			case "attempts":
				out.Attempts = p.integer(f.valueNode, "attempts")
			default:
				p.fail(f.keyNode.Line, "unknown key %q in retry", f.key)
			}
		}
		if out.Name == "" {
			p.fail(e.keyNode.Line, "retry needs an \"of\" component")
		}
		if out.Attempts < 1 || out.Attempts > maxAttempts {
			p.fail(e.keyNode.Line, "retry.attempts must be between 1 and %d", maxAttempts)
		}
	case "fallback":
		out.Kind = KindFallback
		var primary, fb *Node
		hasCoverage := false
		for _, f := range p.entries(e.valueNode, "fallback") {
			switch f.key {
			case "primary":
				primary = p.node(f.valueNode, names, depth+1)
			case "fallback":
				fb = p.node(f.valueNode, names, depth+1)
			case "coverage":
				out.Ratio, hasCoverage = p.ratio(f.valueNode, "coverage"), true
			default:
				p.fail(f.keyNode.Line, "unknown key %q in fallback", f.key)
			}
		}
		if primary == nil || fb == nil || !hasCoverage {
			p.fail(e.keyNode.Line, "fallback needs primary, fallback and coverage")
			return out
		}
		out.Children = []*Node{primary, fb}
	case "cached", "optional":
		out.Kind = map[string]Kind{"cached": KindCached, "optional": KindOptional}[e.key]
		ratioKey := map[string]string{"cached": "hit_ratio", "optional": "touch"}[e.key]
		var of *Node
		hasRatio := false
		for _, f := range p.entries(e.valueNode, e.key) {
			switch f.key {
			case "of":
				of = p.node(f.valueNode, names, depth+1)
			case ratioKey:
				out.Ratio, hasRatio = p.ratio(f.valueNode, ratioKey), true
			default:
				p.fail(f.keyNode.Line, "unknown key %q in %s", f.key, e.key)
			}
		}
		if of == nil || !hasRatio {
			p.fail(e.keyNode.Line, "%s needs \"of\" and %s", e.key, ratioKey)
			return out
		}
		out.Children = []*Node{of}
	}
	return out
}

func (p *parser) list(n *yaml.Node, what string, names map[string]bool, depth int) []*Node {
	if n.Kind != yaml.SequenceNode || len(n.Content) == 0 {
		p.fail(n.Line, "%s needs a non-empty list", what)
		return nil
	}
	out := make([]*Node, len(n.Content))
	for i, c := range n.Content {
		out[i] = p.node(c, names, depth+1)
	}
	return out
}

func (p *parser) ref(n *yaml.Node, names map[string]bool) *Node {
	name, _ := p.scalar(n, "component name")
	if !names[name] {
		p.fail(n.Line, "unknown component %q", name)
	}
	return &Node{Kind: KindRef, Line: n.Line, Name: name}
}

func (p *parser) checkGroups(m *Model) {
	byName := map[string]*Component{}
	for i := range m.Components {
		byName[m.Components[i].Name] = &m.Components[i]
	}
	for i := range m.Components {
		c := &m.Components[i]
		if c.Group != "" && byName[c.Group] == nil {
			p.fail(c.Line, "component %q: group %q is not a component", c.Name, c.Group)
		}
	}
	for i := range m.Components {
		c := &m.Components[i]
		seen := map[string]bool{c.Name: true}
		for g := c.Group; g != "" && byName[g] != nil; g = byName[g].Group {
			if seen[g] {
				p.fail(c.Line, "component %q: group chain contains a cycle through %q", c.Name, g)
				break
			}
			seen[g] = true
		}
	}
}

// warnings reports components that nothing depends on.
func warnings(m *Model) []Warning {
	used := map[string]bool{}
	var walk func(*Node)
	walk = func(n *Node) {
		if n.Kind == KindRef || n.Kind == KindRetry {
			used[n.Name] = true
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(m.Requires)
	for changed := true; changed; {
		changed = false
		for _, c := range m.Components {
			if g := c.Group; used[c.Name] && g != "" && !used[g] {
				used[g], changed = true, true
			}
		}
	}
	var out []Warning
	for _, c := range m.Components {
		if !used[c.Name] {
			out = append(out, Warning{Line: c.Line, Msg: fmt.Sprintf("component %q is never used", c.Name)})
		}
	}
	return out
}
