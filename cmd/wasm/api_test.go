package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func load(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../../testdata/models/" + name + ".yaml")
	require.NoError(t, err)
	return string(b)
}

func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &out), s)
	return out
}

func TestCheck(t *testing.T) {
	out := decode(t, handleCheck(load(t, "shared"), ""))
	require.Equal(t, true, out["ok"])
	res := out["result"].(map[string]any)
	require.Equal(t, float64(1), res["schema_version"])
	require.InDelta(t, 0.9801, res["availability"], 1e-12)
	require.InDelta(t, 0.988119, res["naive"], 1e-12)

	out = decode(t, handleCheck(load(t, "realistic"), `{"uncertainty":true,"seed":7}`))
	require.NotNil(t, out["result"].(map[string]any)["uncertainty"])
}

func TestInvalidModelReturnsLineNumberedErrors(t *testing.T) {
	out := decode(t, handleCheck(load(t, "invalid"), ""))
	require.Equal(t, false, out["ok"])
	errs := out["errors"].([]any)
	require.NotEmpty(t, errs)
	first := errs[0].(map[string]any)
	require.Equal(t, float64(4), first["line"])
}

// The web app ships these as presets, so each must be a valid model.
func TestWebPresetsAreValid(t *testing.T) {
	files, err := filepath.Glob("../../web/src/presets/*.yaml")
	require.NoError(t, err)
	require.Len(t, files, 3)
	for _, f := range files {
		b, err := os.ReadFile(f)
		require.NoError(t, err)
		out := decode(t, handleCheck(string(b), ""))
		require.Equal(t, true, out["ok"], "%s: %v", f, out["errors"])
	}
	b, err := os.ReadFile("../../web/src/presets/fan-out.yaml")
	require.NoError(t, err)
	res := decode(t, handleCheck(string(b), ""))["result"].(map[string]any)
	require.InDelta(t, math.Pow(0.999, 20), res["availability"], 1e-12)
}

func TestBadOptionsAreReported(t *testing.T) {
	out := decode(t, handleCheck(load(t, "shared"), `{`))
	require.Equal(t, false, out["ok"])
}

func TestExplainWhatIfGraph(t *testing.T) {
	out := decode(t, handleExplain(load(t, "shared"), "", ""))
	require.Equal(t, true, out["ok"])
	base := out["result"].(map[string]any)["root"].(map[string]any)["probability"].(float64)
	out = decode(t, handleExplain(load(t, "shared"), `[{"component":"db","percent":99.9}]`, ""))
	require.Equal(t, true, out["ok"])
	require.Greater(t, out["result"].(map[string]any)["root"].(map[string]any)["probability"].(float64), base, "overrides apply before explaining")

	out = decode(t, handleWhatIf(load(t, "checkout"), `[{"component":"payments","percent":99.99}]`, `["db-replica"]`, ""))
	require.Equal(t, true, out["ok"], out)
	require.NotEmpty(t, out["result"].(map[string]any)["rows"])

	out = decode(t, handleWhatIf(load(t, "checkout"), `[{"component":"payments","percent":0.5}]`, "", ""))
	require.Equal(t, false, out["ok"], "a fraction is rejected with the same rules as the file")

	out = decode(t, handleWhatIf(load(t, "checkout"), "", "", ""))
	require.Equal(t, false, out["ok"], "nothing to change")

	out = decode(t, handleGraph(load(t, "operators")))
	require.Equal(t, true, out["ok"])
	require.NotEmpty(t, out["result"].(map[string]any)["nodes"])
}

func TestRemovalsAndUncertaintyReachWhatIfAndExplain(t *testing.T) {
	out := decode(t, handleWhatIf(load(t, "realistic"), "", `["db-replica"]`, `{"uncertainty":true,"seed":3}`))
	require.Equal(t, true, out["ok"], out)
	after := out["result"].(map[string]any)["after"].(map[string]any)
	require.NotNil(t, after["uncertainty"])

	full := decode(t, handleExplain(load(t, "realistic"), "", ""))["result"].(map[string]any)["root"].(map[string]any)["probability"].(float64)
	less := decode(t, handleExplain(load(t, "realistic"), "", `["db-replica"]`))["result"].(map[string]any)["root"].(map[string]any)["probability"].(float64)
	require.Less(t, less, full, "losing the replica lowers the derived availability")
}

func TestEdit(t *testing.T) {
	ops := `[
	  {"op":"add_component","name":"cache","component":{"kind":"point","percent":99.5}},
	  {"op":"add_redundant","name":"cache","to":"db"},
	  {"op":"update_component","name":"a","component":{"kind":"interval","lo":89,"hi":91,"owned":true}}
	]`
	out := decode(t, handleEdit(load(t, "shared"), ops))
	require.Equal(t, true, out["ok"], out)
	edited := out["result"].(map[string]any)["yaml"].(string)
	check := decode(t, handleCheck(edited, ""))
	require.Equal(t, true, check["ok"], edited)
	require.Greater(t, check["result"].(map[string]any)["availability"].(float64), 0.9801, "a redundant db helps")

	bad := decode(t, handleEdit(load(t, "shared"), `[{"op":"add_component","name":"x","component":{"kind":"point","percent":0.5}}]`))
	require.Equal(t, false, bad["ok"])
	bad = decode(t, handleEdit(load(t, "shared"), `[{"op":"explode"}]`))
	require.Equal(t, false, bad["ok"])
	bad = decode(t, handleEdit(load(t, "shared"), `nope`))
	require.Equal(t, false, bad["ok"])
}

func TestGraphCarriesEditorDetails(t *testing.T) {
	out := decode(t, handleGraph(load(t, "operators")))
	nodes := out["result"].(map[string]any)["nodes"].([]any)
	var sawOp, sawComp bool
	for _, n := range nodes {
		node := n.(map[string]any)
		if node["kind"] == "retry" {
			sawOp = true
			require.Equal(t, float64(3), node["params"].(map[string]any)["attempts"])
			require.NotNil(t, node["node"])
		}
		if node["component"] == "billing" {
			sawComp = true
			require.Equal(t, "point", node["info"].(map[string]any)["kind"])
			require.Equal(t, 0.9, node["info"].(map[string]any)["transient_fraction"])
		}
	}
	require.True(t, sawOp && sawComp)
}
