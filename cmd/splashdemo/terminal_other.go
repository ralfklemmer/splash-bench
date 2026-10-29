//go:build !(darwin || linux)

package main

import "os"

// terminalSize is unsupported here; the demo falls back to plain output.
func terminalSize(*os.File) (rows, cols int, ok bool) {
	return 0, 0, false
}
