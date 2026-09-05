//go:build !windows

package platform

import "fmt"

// AttachConsole reports whether this process has a terminal to write to.
//
// Everywhere except Windows the answer is always yes: a program keeps the
// terminal that started it, and one started from a desktop launcher simply has
// its output discarded. Only Windows splits programs into console and GUI kinds
// at link time and needs to be asked.
func AttachConsole() bool { return true }

// ShowMessage has no meaning where there is always a terminal to print to.
func ShowMessage(_, _ string) error {
	return fmt.Errorf("platform: %s always has a terminal; print instead", Current().Display())
}
