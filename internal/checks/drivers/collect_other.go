//go:build !windows

package drivers

import (
	"context"
	"fmt"

	"github.com/ZanOzair/supportone/internal/platform"
)

// collectProblemDevices exists so the package builds everywhere, but the check
// declares Windows as its only platform, so this is never reached through a
// snapshot. It returns an error rather than an empty list: an empty list would
// claim there are no problem devices on a platform that was never asked.
func collectProblemDevices(context.Context, platform.Runner) ([]device, error) {
	return nil, fmt.Errorf("drivers: device error states are a Windows concept; this check does not run on %s",
		platform.Current().Display())
}
