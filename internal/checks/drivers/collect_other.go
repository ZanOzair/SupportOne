//go:build !windows

package drivers

import (
	"context"

	"github.com/ZanOzair/SupportOne/internal/checks"
)

// Run reports that the check does not apply on this platform.
//
// macOS and Linux have no equivalent of a device Windows has flagged as not
// working, so there is nothing to collect and nothing to fail at. The registry
// only offers this check on Windows; if it is reached anyway, it says it has no
// answer rather than reporting that every device is fine on a platform it never
// asked.
func (problemCheck) Run(context.Context) (checks.Result, error) {
	return checks.Unknown(keyDriversNotApplicable), nil
}
