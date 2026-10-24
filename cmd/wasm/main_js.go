//go:build js && wasm

package main

import "syscall/js"

// The core is exposed as globalThis.nines = {check, explain, whatif, edit, graph}. Every function takes strings
// and returns a JSON string (see response in api.go).
func main() {
	arg := func(args []js.Value, i int) string {
		if i < len(args) && args[i].Type() == js.TypeString {
			return args[i].String()
		}
		return ""
	}
	api := map[string]any{
		"check":   js.FuncOf(func(_ js.Value, a []js.Value) any { return handleCheck(arg(a, 0), arg(a, 1)) }),
		"explain": js.FuncOf(func(_ js.Value, a []js.Value) any { return handleExplain(arg(a, 0), arg(a, 1), arg(a, 2)) }),
		"whatif":  js.FuncOf(func(_ js.Value, a []js.Value) any { return handleWhatIf(arg(a, 0), arg(a, 1), arg(a, 2), arg(a, 3)) }),
		"edit":    js.FuncOf(func(_ js.Value, a []js.Value) any { return handleEdit(arg(a, 0), arg(a, 1)) }),
		"graph":   js.FuncOf(func(_ js.Value, a []js.Value) any { return handleGraph(arg(a, 0)) }),
	}
	js.Global().Set("nines", js.ValueOf(api))
	select {} // keep the runtime alive for callbacks
}
