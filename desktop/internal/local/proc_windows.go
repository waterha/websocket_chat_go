//go:build windows

package local

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps service helper processes from flashing a console while the
// launcher runs as a GUI application.
func hideWindow(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
