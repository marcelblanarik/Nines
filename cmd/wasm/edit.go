package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/marcelblanarik/nines/internal/model"
)

// componentJSON is a component as the editor form describes it.
type componentJSON struct {
	Kind              string  `json:"kind"` // point, interval or counts
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

func (c componentJSON) component(name string) (model.Component, error) {
	out := model.Component{Name: name, Owned: c.Owned, Source: c.Source, TransientFraction: c.TransientFraction, Group: c.Group}
	switch c.Kind {
	case "", "point":
		out.Avail = model.PointAvailability(c.Percent)
	case "interval":
		out.Avail = model.Availability{Kind: model.Interval, Lo: c.Lo, Hi: c.Hi}
	case "counts":
		out.Avail = model.Availability{Kind: model.Counts, Good: c.Good, Total: c.Total}
	default:
		return out, fmt.Errorf("unknown availability kind %q", c.Kind)
	}
	return out, nil
}

// editOp is one structural change. Op is one of: add_component, update_component, delete_component,
// add_requirement, add_redundant, add_child, set_node.
type editOp struct {
	Op        string         `json:"op"`
	Name      string         `json:"name"`
	To        string         `json:"to"`
	Index     int            `json:"index"`
	Component *componentJSON `json:"component"`
	Kind      string         `json:"kind"` // set_node: all or any
	K         *int           `json:"k"`
	Attempts  *int           `json:"attempts"`
	Ratio     *float64       `json:"ratio"`
}

func (op editOp) apply(m *model.Model) (*model.Model, error) {
	needComponent := func() (model.Component, error) {
		if op.Component == nil {
			return model.Component{}, errors.New(op.Op + " needs a component")
		}
		return op.Component.component(op.Name)
	}
	switch op.Op {
	case "add_component":
		c, err := needComponent()
		if err != nil {
			return nil, err
		}
		return m.AddComponent(c)
	case "update_component":
		c, err := needComponent()
		if err != nil {
			return nil, err
		}
		return m.UpdateComponent(op.Name, c)
	case "delete_component":
		return m.Without(op.Name)
	case "add_requirement":
		return m.AddRequirement(op.Name)
	case "add_redundant":
		return m.AddRedundant(op.Name, op.To)
	case "add_child":
		return m.AddChild(op.Index, op.Name)
	case "set_node":
		e := model.NodeEdit{K: op.K, Attempts: op.Attempts, Ratio: op.Ratio}
		switch op.Kind {
		case "":
		case "all":
			k := model.KindAll
			e.Kind = &k
		case "any":
			k := model.KindAny
			e.Kind = &k
		default:
			return nil, fmt.Errorf("a node can become all or any, not %q", op.Kind)
		}
		return m.SetNode(op.Index, e)
	}
	return nil, fmt.Errorf("unknown edit %q", op.Op)
}

// handleEdit applies a JSON array of structural edits in order and returns the edited model as YAML. Edits
// rewrite the file in canonical form, so comments are not kept. If any edit fails nothing is returned.
func handleEdit(yamlText, opsJSON string) string {
	var ops []editOp
	if err := json.Unmarshal([]byte(opsJSON), &ops); err != nil {
		return fail(errors.New("invalid edits: " + err.Error()))
	}
	m, err := parse(yamlText)
	if err != nil {
		return fail(err)
	}
	for _, op := range ops {
		if m, err = op.apply(m); err != nil {
			return fail(err)
		}
	}
	out, err := model.Format(m)
	if err != nil {
		return fail(err)
	}
	return encode(response{OK: true, Result: map[string]string{"yaml": string(out)}})
}
