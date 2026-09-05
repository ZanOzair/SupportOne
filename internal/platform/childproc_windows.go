package platform

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// NoConsoleWindow stops a console program from opening a window of its own.
//
// The agent is a windowed program on Windows and so has no console. When a
// process with no console starts a console program, Windows gives that child a
// brand new console window — so every check that runs a system tool flashed a
// black window on screen and took it away again, dozens of times per run. That
// is the cost of the GUI subsystem, and this is the part of it that has to be
// paid explicitly rather than inherited.
//
// CREATE_NO_WINDOW is the exact fix: it runs a console program without a
// console window. Windows ignores the flag for programs that are not console
// programs, so it is safe on everything the agent starts and it does not touch
// any window a windowed program opens for itself.
//
// SysProcAttr.HideWindow is deliberately not used. It sets STARTF_USESHOWWINDOW
// with SW_HIDE, which does apply to windowed programs — it would hide the
// browser the agent is trying to open, which is the opposite of the intent.
//
// Output is unaffected. The agent reads its tools through pipes; nothing it
// needs was ever going to a console.
func NoConsoleWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}
