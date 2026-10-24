// Package bdd implements a reduced ordered binary decision diagram (ROBDD).
// Variable i sits at level i: smaller indexes are closer to the root.
package bdd

import (
	"errors"
	"sort"
)

// Node is a handle into a Manager. False and True are the terminals.
type Node int32

const (
	False Node = 0
	True  Node = 1
)

// ErrNodeLimit is returned by Err once the manager has grown past its node cap.
var ErrNodeLimit = errors.New("bdd: node limit exceeded")

type entry struct {
	v      int32
	lo, hi Node
}

type iteKey struct{ f, g, h Node }

// Manager owns the unique table and the ITE cache. It is not safe for concurrent use.
type Manager struct {
	nodes    []entry
	unique   map[entry]Node
	cache    map[iteKey]Node
	maxNodes int
	err      error
}

// New returns a manager that refuses to grow beyond maxNodes nodes (0 means unlimited).
func New(maxNodes int) *Manager {
	return &Manager{
		nodes:    []entry{{v: -1}, {v: -1}}, // terminals
		unique:   map[entry]Node{},
		cache:    map[iteKey]Node{},
		maxNodes: maxNodes,
	}
}

// Err reports ErrNodeLimit once the cap was hit. After that every result is False and must be discarded.
func (m *Manager) Err() error { return m.err }

// Size is the number of nodes allocated, terminals included.
func (m *Manager) Size() int { return len(m.nodes) }

func (m *Manager) mk(v int32, lo, hi Node) Node {
	if lo == hi {
		return lo
	}
	e := entry{v, lo, hi}
	if n, ok := m.unique[e]; ok {
		return n
	}
	if m.maxNodes > 0 && len(m.nodes) >= m.maxNodes {
		m.err = ErrNodeLimit
		return False
	}
	n := Node(len(m.nodes))
	m.nodes = append(m.nodes, e)
	m.unique[e] = n
	return n
}

// Var returns the function that is true exactly when variable v is true.
func (m *Manager) Var(v int) Node { return m.mk(int32(v), False, True) }

func (m *Manager) top(f Node) int32 {
	if f <= True {
		return 1 << 30
	}
	return m.nodes[f].v
}

func (m *Manager) cofactors(f Node, v int32) (Node, Node) {
	if f <= True || m.nodes[f].v != v {
		return f, f
	}
	return m.nodes[f].lo, m.nodes[f].hi
}

// ITE computes if f then g else h.
func (m *Manager) ITE(f, g, h Node) Node {
	if m.err != nil {
		return False
	}
	switch {
	case f == True:
		return g
	case f == False:
		return h
	case g == h:
		return g
	case g == True && h == False:
		return f
	}
	k := iteKey{f, g, h}
	if r, ok := m.cache[k]; ok {
		return r
	}
	v := min(m.top(f), m.top(g), m.top(h))
	f0, f1 := m.cofactors(f, v)
	g0, g1 := m.cofactors(g, v)
	h0, h1 := m.cofactors(h, v)
	r := m.mk(v, m.ITE(f0, g0, h0), m.ITE(f1, g1, h1))
	if m.err == nil {
		m.cache[k] = r
	}
	return r
}

// And is f AND g.
func (m *Manager) And(f, g Node) Node { return m.ITE(f, g, False) }

// Or is f OR g.
func (m *Manager) Or(f, g Node) Node { return m.ITE(f, True, g) }

// Restrict fixes the given variables to constants and returns the resulting function.
func (m *Manager) Restrict(f Node, assign map[int]bool) Node {
	memo := map[Node]Node{}
	var rec func(Node) Node
	rec = func(n Node) Node {
		if n <= True || m.err != nil {
			return n
		}
		if r, ok := memo[n]; ok {
			return r
		}
		e := m.nodes[n]
		var r Node
		if val, ok := assign[int(e.v)]; ok {
			if val {
				r = rec(e.hi)
			} else {
				r = rec(e.lo)
			}
		} else {
			r = m.mk(e.v, rec(e.lo), rec(e.hi))
		}
		memo[n] = r
		return r
	}
	return rec(f)
}

// Support lists the variables f depends on, ascending.
func (m *Manager) Support(f Node) []int {
	seen := map[Node]bool{}
	vars := map[int]bool{}
	var rec func(Node)
	rec = func(n Node) {
		if n <= True || seen[n] {
			return
		}
		seen[n] = true
		vars[int(m.nodes[n].v)] = true
		rec(m.nodes[n].lo)
		rec(m.nodes[n].hi)
	}
	rec(f)
	out := make([]int, 0, len(vars))
	for v := range vars {
		out = append(out, v)
	}
	sort.Ints(out)
	return out
}

// Plan is a frozen, manager-independent copy of the nodes reachable from one root,
// for repeated probability evaluation with changing inputs. Not safe for concurrent use.
type Plan struct {
	v      []int32
	lo, hi []int32 // indexes into the plan; 0 = False, 1 = True
	root   int32
	vals   []float64
}

// Plan freezes f for repeated evaluation.
func (m *Manager) Plan(f Node) *Plan {
	p := &Plan{v: []int32{-1, -1}, lo: []int32{0, 0}, hi: []int32{0, 0}}
	idx := map[Node]int32{False: 0, True: 1}
	var rec func(Node) int32
	rec = func(n Node) int32 {
		if i, ok := idx[n]; ok {
			return i
		}
		e := m.nodes[n]
		lo, hi := rec(e.lo), rec(e.hi)
		i := int32(len(p.v))
		p.v = append(p.v, e.v)
		p.lo = append(p.lo, lo)
		p.hi = append(p.hi, hi)
		idx[n] = i
		return i
	}
	p.root = rec(f)
	p.vals = make([]float64, len(p.v))
	p.vals[1] = 1
	return p
}

// Probability is P(f is true) when variable i is independently true with probability probs[i]:
// Shannon expansion P(node) = p*P(high) + (1-p)*P(low), evaluated once per node.
func (p *Plan) Probability(probs []float64) float64 {
	for i := 2; i < len(p.v); i++ {
		q := probs[p.v[i]]
		p.vals[i] = q*p.vals[p.hi[i]] + (1-q)*p.vals[p.lo[i]]
	}
	return p.vals[p.root]
}

// Probability evaluates f once; use Plan for repeated evaluation.
func (m *Manager) Probability(f Node, probs []float64) float64 {
	return m.Plan(f).Probability(probs)
}
