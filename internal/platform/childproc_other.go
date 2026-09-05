//go:build !windows

package platform

import "os/exec"

// NoConsoleWindow does nothing here.
//
// Only Windows invents a console window for a console program started by a
// program that has no console. Everywhere else a child process inherits the
// terminal or has none, and neither case puts anything on screen.
func NoConsoleWindow(_ *exec.Cmd) {}

// HideConsoleTool does nothing here, for the same reason.
func HideConsoleTool(_ *exec.Cmd) {}
