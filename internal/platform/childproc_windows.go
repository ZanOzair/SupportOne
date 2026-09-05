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
// It does not set SysProcAttr.HideWindow, which is STARTF_USESHOWWINDOW with
// SW_HIDE. That one does apply to windowed programs, so it would hide the
// browser the agent is trying to open. Use HideConsoleTool where the program
// is known to be a console tool and that risk does not exist.
//
// Output is unaffected. The agent reads its tools through pipes; nothing it
// needs was ever going to a console.
func NoConsoleWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}

// HideConsoleTool suppresses the window of a program known to be a console
// tool. It is NoConsoleWindow with SW_HIDE added.
//
// CREATE_NO_WINDOW is documented as sufficient on its own, and the first
// attempt at this fix used it alone on that basis. A real machine then reported
// the windows still appearing. Rather than argue with the machine, this asks
// twice: the creation flag says do not make a console, and STARTF_USESHOWWINDOW
// with SW_HIDE says that if one is made anyway, do not show it. Windows has
// more than one route to a console window — a redirected default terminal
// application is one — and only one of these covers each.
//
// This is safe here and would not be safe generally. SW_HIDE hides a windowed
// program's window too. Only run() calls this, and run() only ever starts the
// compiled-in console tools the checks and fixes use — powershell, wmic, netsh
// and their equivalents. Anything that might open a window of its own gets
// NoConsoleWindow instead.
func HideConsoleTool(cmd *exec.Cmd) {
	NoConsoleWindow(cmd)
	cmd.SysProcAttr.HideWindow = true
}
