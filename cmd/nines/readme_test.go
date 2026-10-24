package main

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var consoleBlock = regexp.MustCompile("(?s)```console\n(.*?)```")

// trimLines removes trailing spaces on each line and trailing blank lines.
func trimLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// TestReadmeExamples runs every ```console block in README.md ("$ nines ..." followed by its output)
// and fails if the output has drifted.
func TestReadmeExamples(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	require.NoError(t, err)
	blocks := consoleBlock.FindAllStringSubmatch(string(readme), -1)
	require.NotEmpty(t, blocks, "README has no runnable examples")
	for _, b := range blocks {
		cmdLine, want, _ := strings.Cut(b[1], "\n")
		require.True(t, strings.HasPrefix(cmdLine, "$ nines "), "console block must start with $ nines: %q", cmdLine)
		args := strings.Fields(strings.TrimPrefix(cmdLine, "$ nines "))
		for i, a := range args {
			if strings.HasPrefix(a, "testdata/") {
				args[i] = "../../" + a
			}
		}
		t.Run(cmdLine, func(t *testing.T) {
			got, _, _ := execCLI(t, args...)
			require.Equal(t, trimLines(want), trimLines(got))
		})
	}
}
