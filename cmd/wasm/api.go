// Command wasm exposes the nines core to JavaScript. The handlers here take YAML text and return JSON, so
// they are plain Go and testable without a browser; main_js.go registers them on a global.
package main

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/marcelblanarik/nines/internal/analysis"
	"github.com/marcelblanarik/nines/internal/model"
	"github.com/marcelblanarik/nines/internal/report"
)

type apiError struct {
	Line int    `json:"line,omitempty"`
	Msg  string `json:"msg"`
}

// response is the envelope of every handler: a result, or the reasons there is none.
type response struct {
	OK     bool       `json:"ok"`
	Result any        `json:"result,omitempty"`
	Errors []apiError `json:"errors,omitempty"`
}

type checkOptions struct {
	Uncertainty bool   `json:"uncertainty"`
	Seed        uint64 `json:"seed"`
	MaxBDDNodes int    `json:"maxBddNodes"`
}

func encode(r response) string {
	b, err := json.Marshal(r)
	if err != nil {
		return `{"ok":false,"errors":[{"msg":"internal error: could not encode the result"}]}`
	}
	return string(b)
}

func fail(err error) string {
	var list model.ErrorList
	if errors.As(err, &list) {
		out := make([]apiError, len(list))
		for i, e := range list {
			out[i] = apiError{Line: e.Line, Msg: e.Msg}
		}
		return encode(response{Errors: out})
	}
	return encode(response{Errors: []apiError{{Msg: err.Error()}}})
}

func parse(yamlText string) (*model.Model, error) { return model.Parse([]byte(yamlText)) }

func decodeOptions(optsJSON string) (checkOptions, error) {
	opts := checkOptions{Seed: analysis.DefaultSeed}
	if optsJSON != "" {
		if err := json.Unmarshal([]byte(optsJSON), &opts); err != nil {
			return opts, errors.New("invalid options: " + err.Error())
		}
	}
	return opts, nil
}

func (o checkOptions) analysis() analysis.Options {
	return analysis.Options{Uncertainty: o.Uncertainty, Seed: o.Seed, MaxBDDNodes: o.MaxBDDNodes}
}

// handleCheck parses a model and returns the eval.Result. optsJSON may be empty.
func handleCheck(yamlText, optsJSON string) string {
	opts, err := decodeOptions(optsJSON)
	if err != nil {
		return fail(err)
	}
	m, err := parse(yamlText)
	if err != nil {
		return fail(err)
	}
	res, err := analysis.Check(m, opts.analysis())
	if err != nil {
		return fail(err)
	}
	return encode(response{OK: true, Result: res})
}

// handleExplain returns the derivation trace, optionally with removals and {component, percent} overrides
// applied first (the same what-if the sliders use).
func handleExplain(yamlText, setsJSON, removesJSON string) string {
	sets, removes, err := whatIfArgs(setsJSON, removesJSON)
	if err != nil {
		return fail(err)
	}
	m, err := parse(yamlText)
	if err != nil {
		return fail(err)
	}
	for _, name := range removes {
		if m, err = m.Without(name); err != nil {
			return fail(err)
		}
	}
	if m, err = applySets(m, sets); err != nil {
		return fail(err)
	}
	t, err := analysis.Explain(m, analysis.Options{})
	if err != nil {
		return fail(err)
	}
	return encode(response{OK: true, Result: t})
}

// whatIfArgs decodes the JSON arrays the what-if style handlers take; either may be empty.
func whatIfArgs(setsJSON, removesJSON string) ([]analysis.Set, []string, error) {
	var sets []analysis.Set
	var removes []string
	if setsJSON != "" {
		if err := json.Unmarshal([]byte(setsJSON), &sets); err != nil {
			return nil, nil, errors.New("invalid sets: " + err.Error())
		}
	}
	if removesJSON != "" {
		if err := json.Unmarshal([]byte(removesJSON), &removes); err != nil {
			return nil, nil, errors.New("invalid removes: " + err.Error())
		}
	}
	return sets, removes, nil
}

// applySets returns a copy of m with each override applied, validating percentages like the file does.
func applySets(m *model.Model, sets []analysis.Set) (*model.Model, error) {
	if err := validatePercents(sets); err != nil {
		return nil, err
	}
	for _, s := range sets {
		var err error
		if m, err = m.WithAvailability(s.Component, model.PointAvailability(s.Percent)); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func validatePercents(sets []analysis.Set) error {
	for _, s := range sets {
		if _, err := model.ParsePercent(strconv.FormatFloat(s.Percent, 'g', -1, 64)); err != nil {
			return errors.New("set " + s.Component + ": " + err.Error())
		}
	}
	return nil
}

// handleWhatIf applies {component, percent} overrides and removals (JSON arrays) and returns the diff against
// the baseline. optsJSON takes the same options as check (uncertainty, seed). Every argument after the YAML
// may be empty.
func handleWhatIf(yamlText, setsJSON, removesJSON, optsJSON string) string {
	sets, removes, err := whatIfArgs(setsJSON, removesJSON)
	if err != nil {
		return fail(err)
	}
	opts, err := decodeOptions(optsJSON)
	if err != nil {
		return fail(err)
	}
	if err := validatePercents(sets); err != nil {
		return fail(err)
	}
	m, err := parse(yamlText)
	if err != nil {
		return fail(err)
	}
	d, err := analysis.WhatIf(m, sets, removes, opts.analysis())
	if err != nil {
		return fail(err)
	}
	return encode(response{OK: true, Result: d})
}

// handleGraph returns the nodes and edges for drawing the model.
func handleGraph(yamlText string) string {
	m, err := parse(yamlText)
	if err != nil {
		return fail(err)
	}
	return encode(response{OK: true, Result: report.Graph(m)})
}
