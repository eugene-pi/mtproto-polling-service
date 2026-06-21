//go:build !windows

package browser

import (
	"os/exec"
	"runtime"
)

// open launches the platform's default URL handler. The interactive flag is
// only meaningful on Windows and is ignored here.
func open(url string, _ bool) error {
	name, args := consoleCommand(url)
	return exec.Command(name, args...).Start()
}

// consoleCommand returns the command that opens url in the default browser.
func consoleCommand(url string) (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{url}
	default:
		return "xdg-open", []string{url}
	}
}
