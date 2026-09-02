package platform

import "golang.org/x/sys/windows"

// IsElevated reports whether the current process holds administrator rights.
// The agent never requires this to start; it is consulted per action so a
// module that needs elevation can say so instead of failing obscurely.
func IsElevated() (bool, error) {
	return windows.GetCurrentProcessToken().IsElevated(), nil
}
