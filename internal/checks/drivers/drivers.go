// Package drivers reports devices Windows has flagged as not working.
//
// The check registers on Windows only. macOS and Linux have no equivalent
// notion of a device in an error state that a user would recognise as "a driver
// problem", and inventing one would mean reporting a verdict the OS never gave.
// On those platforms the report says the check is unavailable here.
package drivers

import (
	"context"
	"strings"

	"github.com/ZanOzair/supportone/internal/checks"
	"github.com/ZanOzair/supportone/internal/platform"
)

// device is one entry Device Manager would show with a warning triangle.
type device struct {
	Name      string `json:"name"`
	DeviceID  string `json:"device_id,omitempty"`
	ErrorCode int    `json:"error_code"`
	Meaning   string `json:"meaning,omitempty"`
}

// Message keys for this package's results.
const (
	keyDriversOK      = "check.drivers.problem.ok"
	keyDriversProblem = "check.drivers.problem.found"
)

type problemCheck struct{ run platform.Runner }

func (problemCheck) ID() string               { return "drivers.problem" }
func (problemCheck) Platforms() []platform.OS { return []platform.OS{platform.Windows} }
func (problemCheck) RequiresAdmin() bool      { return false }

func (c problemCheck) Run(ctx context.Context) (checks.Result, error) {
	devices, err := collectProblemDevices(ctx, c.run)
	if err != nil {
		return checks.UnknownFor(err), nil
	}
	return problemVerdict(devices), nil
}

// problemVerdict names the devices Windows says are not working. It is
// separate from collection so it can be tested on any machine.
func problemVerdict(devices []device) checks.Result {
	if len(devices) == 0 {
		return checks.OK(keyDriversOK)
	}

	names := make([]string, 0, len(devices))
	for _, d := range devices {
		names = append(names, d.Name)
	}
	return checks.Attention(checks.PluralKey(keyDriversProblem, len(devices)), len(devices), strings.Join(names, ", ")).
		With(map[string]any{"devices": devices})
}

func init() {
	checks.MustRegister(problemCheck{run: platform.RunRead})
}
