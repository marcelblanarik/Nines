package compile

import (
	"fmt"

	"github.com/marcelblanarik/nines/internal/model"
)

// Role says what a variable stands for.
type Role uint8

const (
	// RoleHard is a component's persistent failure, shared by every reference to it.
	RoleHard Role = iota
	// RoleSoft is a component's transient failure, fresh per call site.
	RoleSoft
	// RoleOp is a fixed-probability variable introduced by an operator (coverage, hit ratio, touch).
	RoleOp
)

// Var is one independent Bernoulli variable (true means up).
type Var struct {
	ID        int
	Name      string
	Component string // owning component; empty for RoleOp
	Role      Role
	Prob      float64 // RoleOp only
}

// TNode mirrors one requires-tree node with the formula it compiled to.
type TNode struct {
	Node     *model.Node
	F        *Formula
	Children []*TNode
}

// Compiled is a model turned into a formula.
type Compiled struct {
	Model *model.Model
	Vars  []Var
	Tree  *TNode
	Root  *Formula
	// ComponentVars lists the variables each component owns, in creation order.
	ComponentVars map[string][]int
	// Components lists the components that own variables, in order of first appearance.
	Components []string
}

// Options tweak compilation.
type Options struct {
	// Naive gives every occurrence of a component (and of a group reference) its own variables,
	// which is what multiplying percentages down the tree implicitly does.
	Naive bool
}

type builder struct {
	opt   Options
	comps map[string]*model.Component
	c     *Compiled
	hard  map[string]int
	seq   map[string]int
}

// Compile builds the formula for m. Variables are numbered by first appearance in a depth-first walk
// of requires, which is also the BDD variable order.
func Compile(m *model.Model, opt Options) (*Compiled, error) {
	b := &builder{
		opt:   opt,
		comps: map[string]*model.Component{},
		c:     &Compiled{Model: m, ComponentVars: map[string][]int{}},
		hard:  map[string]int{},
		seq:   map[string]int{},
	}
	for i := range m.Components {
		b.comps[m.Components[i].Name] = &m.Components[i]
	}
	t, err := b.node(m.Requires)
	if err != nil {
		return nil, err
	}
	b.c.Tree, b.c.Root = t, t.F
	return b.c, nil
}

func (b *builder) newVar(name, comp string, role Role, prob float64) int {
	id := len(b.c.Vars)
	b.c.Vars = append(b.c.Vars, Var{ID: id, Name: name, Component: comp, Role: role, Prob: prob})
	if comp != "" {
		if len(b.c.ComponentVars[comp]) == 0 {
			b.c.Components = append(b.c.Components, comp)
		}
		b.c.ComponentVars[comp] = append(b.c.ComponentVars[comp], id)
	}
	return id
}

func (b *builder) next(key string) int {
	b.seq[key]++
	return b.seq[key]
}

func (b *builder) hardVar(comp string) int {
	if id, ok := b.hard[comp]; ok && !b.opt.Naive {
		return id
	}
	name := comp + ".hard"
	if b.opt.Naive {
		name = fmt.Sprintf("%s.hard@%d", comp, b.next(comp+"/hard"))
	}
	id := b.newVar(name, comp, RoleHard, 0)
	b.hard[comp] = id
	return id
}

func (b *builder) opVar(kind string, prob float64) *Formula {
	n := b.next("op/" + kind)
	return v(b.newVar(fmt.Sprintf("%s#%d", kind, n), "", RoleOp, prob))
}

func (b *builder) group(g string) *Formula {
	f := v(b.hardVar(g))
	if up := b.comps[g].Group; up != "" {
		return and(f, b.group(up))
	}
	return f
}

// call compiles one call site of a component: its persistent state, one or more transient
// attempts, and the group it lives in.
func (b *builder) call(name string, attempts int) (*Formula, error) {
	c := b.comps[name]
	if c == nil {
		return nil, fmt.Errorf("unknown component %q", name)
	}
	parts := []*Formula{v(b.hardVar(name))}
	if c.TransientFraction > 0 {
		softs := make([]*Formula, attempts)
		for i := range softs {
			softs[i] = v(b.newVar(fmt.Sprintf("%s.soft#%d", name, b.next(name+"/soft")), name, RoleSoft, 0))
		}
		parts = append(parts, or(softs...))
	}
	if c.Group != "" {
		parts = append(parts, b.group(c.Group))
	}
	return and(parts...), nil
}

func (b *builder) node(n *model.Node) (*TNode, error) {
	t := &TNode{Node: n}
	switch n.Kind {
	case model.KindRef:
		f, err := b.call(n.Name, 1)
		t.F = f
		return t, err
	case model.KindRetry:
		f, err := b.call(n.Name, n.Attempts)
		t.F = f
		return t, err
	}
	for _, c := range n.Children {
		ct, err := b.node(c)
		if err != nil {
			return nil, err
		}
		t.Children = append(t.Children, ct)
	}
	args := make([]*Formula, len(t.Children))
	for i, c := range t.Children {
		args[i] = c.F
	}
	switch n.Kind {
	case model.KindAll:
		t.F = and(args...)
	case model.KindAny:
		t.F = or(args...)
	case model.KindKOfN:
		t.F = &Formula{Kind: FKOfN, K: n.K, Args: args}
	case model.KindFallback:
		t.F = or(args[0], and(args[1], b.opVar("fallback.coverage", n.Ratio)))
	case model.KindCached:
		t.F = or(b.opVar("cache.hit", n.Ratio), args[0])
	case model.KindOptional:
		t.F = or(b.opVar("optional.untouched", 1-n.Ratio), args[0])
	default:
		return nil, fmt.Errorf("line %d: unsupported node kind %d", n.Line, n.Kind)
	}
	return t, nil
}

// PointAvail maps every component to its point-estimate availability (a fraction).
func (c *Compiled) PointAvail() map[string]float64 {
	out := make(map[string]float64, len(c.Model.Components))
	for _, comp := range c.Model.Components {
		out[comp.Name] = comp.Avail.Value()
	}
	return out
}

// Probs returns each variable's up-probability. Components missing from avail use their point estimate.
//
// With U = 1-A and transient fraction r: hard = 1-(1-r)U and soft = 1-q where q = rU/hard,
// so one call succeeds with hard*(1-q) = A.
func (c *Compiled) Probs(avail map[string]float64) []float64 {
	r := map[string]float64{}
	for _, comp := range c.Model.Components {
		r[comp.Name] = comp.TransientFraction
	}
	point := c.PointAvail()
	out := make([]float64, len(c.Vars))
	for i, vr := range c.Vars {
		if vr.Role == RoleOp {
			out[i] = vr.Prob
			continue
		}
		a, ok := avail[vr.Component]
		if !ok {
			a = point[vr.Component]
		}
		u := 1 - a
		hard := 1 - (1-r[vr.Component])*u
		if vr.Role == RoleHard {
			out[i] = hard
		} else {
			out[i] = 1 - r[vr.Component]*u/hard
		}
	}
	return out
}
