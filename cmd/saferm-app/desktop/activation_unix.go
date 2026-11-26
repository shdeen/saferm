//go:build darwin || linux

package desktop

// ForwardActivation forwards nothing and reports false. On macOS and Linux,
// Wails hands a launch to the running instance by itself.
func ForwardActivation(_ string) (bool, error) {
	return false, nil
}
