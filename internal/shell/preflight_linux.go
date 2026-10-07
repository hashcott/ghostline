package shell

import (
	"fmt"
	"os"
)

// preflight has nothing to check on Linux: the package manager installs
// WebKitGTK with Ghostline.
func preflight() error { return nil }

// fatalBox reports a startup failure on the terminal.
func fatalBox(err error) {
	fmt.Fprintf(os.Stderr, "Ghostline không thể khởi động / could not start: %v\n", err)
}
