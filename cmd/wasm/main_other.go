//go:build !(js && wasm)

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "this command only runs as WebAssembly: build it with GOOS=js GOARCH=wasm (see make wasm)")
	os.Exit(1)
}
