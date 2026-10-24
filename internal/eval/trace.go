package eval

import (
	"errors"
	"fmt"
	"sort"

	"github.com/marcelblanarik/nines/internal/compile"
	"github.com/marcelblanarik/nines/internal/model"
)

// Trace mirrors the requires tree. Every node carries the exact probability of its own sub-formula,
// computed from that sub-formula's BDD rather than by multiplying its children.
func (e *Engine) Trace(probs []float64) (*Trace, error) {
	root := e.traceNode(e.C.Tree, probs)
	if err := e.M.Err(); err != nil {
		return nil, errors.Join(errors.New("derivation trace"), err)
	}
	return &Trace{SchemaVersion: SchemaVersion, Service: e.C.Model.Service, Root: root}, nil
}

var kindNames = map[model.Kind]string{
	model.KindRef:      "component",
	model.KindAll:      "all",
	model.KindAny:      "any",
	model.KindKOfN:     "k_of_n",
	model.KindRetry:    "retry",
	model.KindFallback: "fallback",
	model.KindCached:   "cached",
	model.KindOptional: "optional",
}

func (e *Engine) traceNode(t *compile.TNode, probs []float64) *TraceNode {
	out := &TraceNode{
		Kind:        kindNames[t.Node.Kind],
		Label:       e.label(t.Node),
		Probability: e.M.Probability(e.Node(t.F), probs),
		Shared:      e.shared(t),
	}
	for _, c := range t.Children {
		out.Children = append(out.Children, e.traceNode(c, probs))
	}
	return out
}

// shared lists components whose persistent state is used under two or more children of t.
func (e *Engine) shared(t *compile.TNode) []string {
	if len(t.Children) < 2 {
		return nil
	}
	count := map[int]int{}
	for _, c := range t.Children {
		for v := range compile.Support(c.F) {
			count[v]++
		}
	}
	names := map[string]bool{}
	for v, n := range count {
		if comp := e.C.Vars[v].Component; n > 1 && comp != "" {
			names[comp] = true
		}
	}
	out := make([]string, 0, len(names))
	for n := range names {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func pct(f float64) string { return fmt.Sprintf("%g%%", f*100) }

// label describes a node in plain words.
func (e *Engine) label(n *model.Node) string {
	switch n.Kind {
	case model.KindRef:
		s := fmt.Sprintf("%s works", n.Name)
		if g := e.C.Model.Lookup(n.Name).Group; g != "" {
			s += fmt.Sprintf(" (and so does %s, which it depends on)", g)
		}
		return s
	case model.KindAll:
		return "ALL of these must work"
	case model.KindAny:
		return "AT LEAST ONE of these must work"
	case model.KindKOfN:
		return fmt.Sprintf("AT LEAST %d of these %d must work", n.K, len(n.Children))
	case model.KindRetry:
		s := fmt.Sprintf("%s works, trying up to %d times", n.Name, n.Attempts)
		if e.C.Model.Lookup(n.Name).TransientFraction == 0 {
			s += " (no transient failures declared, so retrying does not help)"
		}
		return s
	case model.KindFallback:
		return fmt.Sprintf("the first works, OR the second works and can serve the request (it can serve %s of requests)", pct(n.Ratio))
	case model.KindCached:
		return fmt.Sprintf("served from cache (%s of requests), OR the origin works", pct(n.Ratio))
	case model.KindOptional:
		return fmt.Sprintf("only needed by %s of requests; otherwise it does not matter", pct(n.Ratio))
	}
	return ""
}
