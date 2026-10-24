package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/marcelblanarik/nines/internal/analysis"
	"github.com/marcelblanarik/nines/internal/eval"
	"github.com/marcelblanarik/nines/internal/model"
	"github.com/marcelblanarik/nines/internal/report"
)

func newRootCmd(stdin io.Reader) *cobra.Command {
	root := &cobra.Command{
		Use:           "nines",
		Short:         "A dependency-aware SLO calculator",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		newCheckCmd(stdin),
		newExplainCmd(stdin),
		newWhatIfCmd(stdin),
		newSolveCmd(stdin),
		newGraphCmd(stdin),
		newFmtCmd(stdin),
		&cobra.Command{
			Use:   "version",
			Short: "Print the version",
			Args:  cobra.NoArgs,
			RunE:  func(cmd *cobra.Command, _ []string) error { return emit(cmd, "nines "+version+"\n") },
		},
	)
	return root
}

// loadModel reads and validates a model file ("-" reads stdin). Validation errors are written to stderr
// as path:line: message and surface as exit code 2.
func loadModel(cmd *cobra.Command, stdin io.Reader, path string) (*model.Model, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, &exitError{exitRuntime, err}
	}
	m, err := model.Parse(data)
	if err != nil {
		var list model.ErrorList
		if errors.As(err, &list) {
			for _, e := range list {
				if e.Line > 0 {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s:%d: %s\n", path, e.Line, e.Msg)
				} else {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", path, e.Msg)
				}
			}
			return nil, &exitError{code: exitInvalid}
		}
		return nil, &exitError{exitInvalid, err}
	}
	return m, nil
}

// emit writes command output to stdout; a failed write (for example a closed pipe) is a runtime error.
func emit(cmd *cobra.Command, s string) error {
	_, err := io.WriteString(cmd.OutOrStdout(), s)
	return runtimeErr(err)
}

func runtimeErr(err error) error {
	if err == nil {
		return nil
	}
	return &exitError{exitRuntime, err}
}

func newCheckCmd(stdin io.Reader) *cobra.Command {
	var (
		format      string
		uncertainty bool
		seed        uint64
		noFail      bool
		maxNodes    int
		samples     int
	)
	cmd := &cobra.Command{
		Use:   "check model.yaml",
		Short: "Compute the availability you can promise, the ceiling, levers and a verdict",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := loadModel(cmd, stdin, args[0])
			if err != nil {
				return err
			}
			res, err := analysis.Check(m, analysis.Options{Uncertainty: uncertainty, Seed: seed, MaxBDDNodes: maxNodes, MCSamples: samples})
			if err != nil {
				return runtimeErr(err)
			}
			out, err := report.Check(res, format)
			if err != nil {
				return runtimeErr(err)
			}
			if err := emit(cmd, out); err != nil {
				return err
			}
			if res.Verdict == eval.Infeasible && !noFail {
				return &exitError{code: exitInfeasible}
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&format, "format", "table", "output format: table, json or md")
	f.BoolVar(&uncertainty, "uncertainty", false, "sample component uncertainty and report percentiles")
	f.Uint64Var(&seed, "seed", analysis.DefaultSeed, "random seed for sampling")
	f.BoolVar(&noFail, "no-fail", false, "exit 0 even when the verdict is infeasible")
	f.IntVar(&maxNodes, "max-bdd-nodes", eval.DefaultMaxBDDNodes, "BDD size cap before falling back to Monte Carlo")
	f.IntVar(&samples, "mc-samples", analysis.DefaultMCSamples, "Monte Carlo samples when falling back")
	return cmd
}

func jsonOrText(format string, v any, text func() string) (string, error) {
	switch format {
	case report.FormatJSON:
		return report.JSON(v)
	case report.FormatTable, "":
		return text(), nil
	}
	return "", fmt.Errorf("unknown format %q (use table or json)", format)
}

func newExplainCmd(stdin io.Reader) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "explain model.yaml",
		Short: "Show, step by step, how the availability is derived",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := loadModel(cmd, stdin, args[0])
			if err != nil {
				return err
			}
			t, err := analysis.Explain(m, analysis.Options{})
			if err != nil {
				return runtimeErr(err)
			}
			out, err := jsonOrText(format, t, func() string { return report.Trace(t) })
			if err != nil {
				return runtimeErr(err)
			}
			return emit(cmd, out)
		},
	}
	cmd.Flags().StringVar(&format, "format", "table", "output format: table or json")
	return cmd
}

func parseSets(specs []string) ([]analysis.Set, error) {
	var out []analysis.Set
	for _, s := range specs {
		name, value, ok := strings.Cut(s, "=")
		if !ok || name == "" {
			return nil, fmt.Errorf("--set %q: want name=value", s)
		}
		p, err := model.ParsePercent(value)
		if err != nil {
			return nil, fmt.Errorf("--set %s: %w", s, err)
		}
		out = append(out, analysis.Set{Component: name, Percent: p})
	}
	return out, nil
}

func newWhatIfCmd(stdin io.Reader) *cobra.Command {
	var (
		sets, removes []string
		format        string
	)
	cmd := &cobra.Command{
		Use:   "whatif model.yaml --set name=99.99 --remove name",
		Short: "Change availabilities or remove components and compare with the baseline",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := loadModel(cmd, stdin, args[0])
			if err != nil {
				return err
			}
			ss, err := parseSets(sets)
			if err != nil {
				return &exitError{exitRuntime, err}
			}
			d, err := analysis.WhatIf(m, ss, removes, analysis.Options{})
			if err != nil {
				return runtimeErr(err)
			}
			out, err := jsonOrText(format, d, func() string { return report.Diff(d) })
			if err != nil {
				return runtimeErr(err)
			}
			return emit(cmd, out)
		},
	}
	f := cmd.Flags()
	f.StringArrayVar(&sets, "set", nil, "set a component's availability, as name=percent (repeatable)")
	f.StringArrayVar(&removes, "remove", nil, "remove a component from the model (repeatable)")
	f.StringVar(&format, "format", "table", "output format: table or json")
	return cmd
}

func newSolveCmd(stdin io.Reader) *cobra.Command {
	var (
		component string
		format    string
	)
	cmd := &cobra.Command{
		Use:   "solve model.yaml --component name",
		Short: "Find the availability a component needs for the system to meet the target",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := loadModel(cmd, stdin, args[0])
			if err != nil {
				return err
			}
			s, err := analysis.Solve(m, component, analysis.Options{})
			if err != nil {
				return runtimeErr(err)
			}
			out, err := jsonOrText(format, s, func() string { return report.Solve(s) })
			if err != nil {
				return runtimeErr(err)
			}
			return emit(cmd, out)
		},
	}
	cmd.Flags().StringVar(&component, "component", "", "component to solve for")
	cmd.Flags().StringVar(&format, "format", "table", "output format: table or json")
	_ = cmd.MarkFlagRequired("component")
	return cmd
}

func newGraphCmd(stdin io.Reader) *cobra.Command {
	return &cobra.Command{
		Use:   "graph model.yaml",
		Short: "Print the dependency graph as Mermaid, annotated with availabilities",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := loadModel(cmd, stdin, args[0])
			if err != nil {
				return err
			}
			return emit(cmd, report.Mermaid(m))
		},
	}
}

func newFmtCmd(stdin io.Reader) *cobra.Command {
	var write bool
	cmd := &cobra.Command{
		Use:   "fmt model.yaml",
		Short: "Validate a model and print it in canonical form (comments are not kept)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := loadModel(cmd, stdin, args[0])
			if err != nil {
				return err
			}
			out, err := model.Format(m)
			if err != nil {
				return runtimeErr(err)
			}
			if write {
				if args[0] == "-" {
					return &exitError{exitRuntime, errors.New("--write needs a file, not stdin")}
				}
				return runtimeErr(os.WriteFile(args[0], out, 0o644))
			}
			_, err = cmd.OutOrStdout().Write(out)
			return runtimeErr(err)
		},
	}
	cmd.Flags().BoolVarP(&write, "write", "w", false, "rewrite the file in place")
	return cmd
}
