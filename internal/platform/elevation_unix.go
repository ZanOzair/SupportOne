//go:build !windows

package platform

import "os"

// IsElevated reports whether the current process is running as root.
func IsElevated() (bool, error) {
	return os.Geteuid() == 0, nil
}
