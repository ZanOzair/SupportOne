// Package platform is the OS abstraction layer. Everything that differs
// between Windows, macOS and Linux is named here so the rest of the codebase
// can stay platform-neutral.
package platform

import (
	"fmt"
	"runtime"
)

// OS identifies a supported desktop operating system.
type OS string

// The desktop operating systems the agent targets.
const (
	Windows OS = "windows"
	Darwin  OS = "darwin"
	Linux   OS = "linux"
)

// All returns every OS SupportOne targets.
func All() []OS { return []OS{Windows, Darwin, Linux} }

// Current returns the OS the agent is running on. It returns an empty OS on
// platforms SupportOne does not target, so callers can refuse to run rather
// than guess.
func Current() OS {
	switch runtime.GOOS {
	case "windows":
		return Windows
	case "darwin":
		return Darwin
	case "linux":
		return Linux
	default:
		return ""
	}
}

// Valid reports whether o is a targeted OS.
func (o OS) Valid() bool {
	return o == Windows || o == Darwin || o == Linux
}

// Display returns the human-facing name of the OS.
func (o OS) Display() string {
	switch o {
	case Windows:
		return "Windows"
	case Darwin:
		return "macOS"
	case Linux:
		return "Linux"
	default:
		return string(o)
	}
}

// Host describes the machine the agent is running on. It carries only
// non-identifying facts; anything identifying belongs in a check result where
// the user can see and redact it before sending.
type Host struct {
	OS   OS     `json:"os"`
	Arch string `json:"arch"`
}

// CurrentHost returns the running machine's OS and architecture.
func CurrentHost() Host {
	return Host{OS: Current(), Arch: runtime.GOARCH}
}

// ErrUnsupportedOS is returned when the agent is started on an OS it does not
// target. Modules degrade gracefully; the agent itself does not pretend.
var ErrUnsupportedOS = fmt.Errorf("supportone: unsupported operating system %q", runtime.GOOS)
