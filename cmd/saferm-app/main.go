// Command saferm-app opens the SafeRM desktop interface.
package main

import (
	"fmt"
	"os"
)

// main runs the desktop application. On failure, it writes the error to
// standard error and exits with status 1.
func main() {
	if err := runApplication(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
