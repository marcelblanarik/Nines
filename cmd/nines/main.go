// Command nines is a dependency-aware SLO calculator.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

// Exit codes. Check uses exitInfeasible so CI can gate on the verdict.
const (
	exitOK         = 0
	exitRuntime    = 1
	exitInvalid    = 2
	exitInfeasible = 3
)

// exitError carries an exit code out of a command after its output has been written.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *exitError) Unwrap() error { return e.err }

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

// run executes the CLI and returns the process exit code.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cmd := newRootCmd(stdin)
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	err := cmd.Execute()
	if err == nil {
		return exitOK
	}
	var ee *exitError
	if errors.As(err, &ee) {
		if msg := ee.Error(); msg != "" {
			_, _ = fmt.Fprintln(stderr, msg)
		}
		return ee.code
	}
	_, _ = fmt.Fprintln(stderr, "error:", err)
	return exitRuntime
}
