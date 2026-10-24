package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "rewrite golden files")

const modelsDir = "../../testdata/models/"

// execCLI runs the CLI and returns stdout, stderr and the exit code.
func execCLI(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, strings.NewReader(""), &out, &errOut)
	return out.String(), errOut.String(), code
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("../../testdata/golden", name+".golden")
	if *update {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "missing golden file; run go test ./cmd/nines -update")
	require.Equal(t, string(want), got)
}

func TestGoldenOutputs(t *testing.T) {
	models := []string{"bridge", "checkout", "fanout", "operators", "realistic", "shared"}
	for _, m := range models {
		file := modelsDir + m + ".yaml"
		t.Run(m, func(t *testing.T) {
			for _, c := range []struct {
				name string
				args []string
			}{
				{"check", []string{"check", file, "--no-fail"}},
				{"check-json", []string{"check", file, "--no-fail", "--format", "json"}},
				{"check-md", []string{"check", file, "--no-fail", "--format", "md"}},
				{"check-uncertainty", []string{"check", file, "--no-fail", "--uncertainty", "--seed", "7"}},
				{"explain", []string{"explain", file}},
				{"graph", []string{"graph", file}},
			} {
				out, errOut, code := execCLI(t, c.args...)
				require.Equal(t, 0, code, errOut)
				golden(t, m+"."+c.name, out)
			}
		})
	}

	out, _, code := execCLI(t, "whatif", modelsDir+"checkout.yaml", "--set", "payments=99.99", "--set", "auth=99.99", "--remove", "db-replica")
	require.Equal(t, 0, code)
	golden(t, "checkout.whatif", out)

	out, _, code = execCLI(t, "solve", modelsDir+"checkout.yaml", "--component", "auth")
	require.Equal(t, 0, code)
	golden(t, "checkout.solve", out)

	out, _, code = execCLI(t, "solve", modelsDir+"checkout.yaml", "--component", "db-primary")
	require.Equal(t, 0, code)
	golden(t, "checkout.solve-no-effect", out)

	out, _, code = execCLI(t, "fmt", modelsDir+"realistic.yaml")
	require.Equal(t, 0, code)
	golden(t, "realistic.fmt", out)
}

func TestExitCodes(t *testing.T) {
	_, _, code := execCLI(t, "check", modelsDir+"checkout.yaml")
	require.Equal(t, exitInfeasible, code)
	_, _, code = execCLI(t, "check", modelsDir+"checkout.yaml", "--no-fail")
	require.Equal(t, exitOK, code)
	_, _, code = execCLI(t, "check", modelsDir+"shared.yaml")
	require.Equal(t, exitInfeasible, code, "98.01%% is below the 99%% target")
}

func TestInvalidModelReportsLines(t *testing.T) {
	_, errOut, code := execCLI(t, "check", modelsDir+"invalid.yaml")
	require.Equal(t, exitInvalid, code)
	require.Contains(t, errOut, "invalid.yaml:4: ")
	require.Contains(t, errOut, "did you mean 99.95?")
	require.Contains(t, errOut, "invalid.yaml:9: unknown component")

	_, _, code = execCLI(t, "check", modelsDir+"missing-file.yaml")
	require.Equal(t, exitRuntime, code)
}

func TestFanOutIsAbout98Percent(t *testing.T) {
	out, _, code := execCLI(t, "check", modelsDir+"fanout.yaml")
	require.Equal(t, exitInfeasible, code)
	require.Contains(t, out, "98.02%")
}

func TestVersion(t *testing.T) {
	out, _, code := execCLI(t, "version")
	require.Equal(t, 0, code)
	require.Equal(t, "nines dev\n", out)
}
