// Command saferm-notify runs SafeRM Notify, the optional background program that
// shows notifications for actions awaiting the user's approval.
package main

import (
	"os"

	"github.com/shdeen/saferm/cmd/saferm-app/desktop"
)

// main runs SafeRM Notify and exits with the exit code that it returns. On
// failure, it logs the error and exits with status 1.
func main() {
	exitCode, err := runNotifier()
	if err != nil {
		desktop.LogError(err)
		os.Exit(1)
	}

	os.Exit(exitCode)
}
