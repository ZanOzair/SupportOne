package restore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

// Compiled-in queries; see platform.RunRead for why this is not shell
// construction.
const (
	psExe = "powershell"

	// System Restore has to be switched on for the system drive before a
	// checkpoint can be made, and the setting is readable without elevation.
	querySystemRestoreEnabled = `$d = Get-CimInstance -Namespace root\default -ClassName SystemRestoreConfig ` +
		`-ErrorAction SilentlyContinue; ` +
		`$p = Get-ComputerRestorePoint -ErrorAction SilentlyContinue | Select-Object -First 1; ` +
		`[pscustomobject]@{Configured=($null -ne $d -or $null -ne $p)} | ConvertTo-Json -Compress`

	// Checkpoint-Computer needs administrator rights. MODIFY_SETTINGS is the
	// checkpoint type Windows uses for a configuration change.
	createCheckpoint = `Checkpoint-Computer -Description %q -RestorePointType MODIFY_SETTINGS; ` +
		`Get-ComputerRestorePoint | Sort-Object CreationTime -Descending | ` +
		`Select-Object -First 1 SequenceNumber | ConvertTo-Json -Compress`
)

// windowsMaker creates Windows System Restore checkpoints.
type windowsMaker struct{ run platform.Runner }

// New returns the restore point maker for this platform.
func New() Maker { return windowsMaker{run: platform.RunRead} }

func (m windowsMaker) Check(ctx context.Context) Availability {
	out := Availability{Kind: "Windows System Restore"}

	elevated, err := platform.IsElevated()
	if err == nil && !elevated {
		out.Reason = KeyUnavailableNeedsAdmin
		return out
	}

	raw, err := m.run(ctx, psExe, "-NoProfile", "-NonInteractive", "-Command", querySystemRestoreEnabled)
	if err != nil {
		out.Reason = KeyUnavailableUnreadable
		return out
	}
	if !strings.Contains(string(raw), `"Configured":true`) {
		// The feature exists but is switched off for this machine. That is a
		// setting the user can change; it is not our place to change it for
		// them as a side effect of a repair.
		out.Reason = KeyUnavailableDisabled
		return out
	}

	out.Available = true
	return out
}

func (m windowsMaker) Create(ctx context.Context, label string) (Point, error) {
	// The label is the only variable, and it is quoted into a PowerShell
	// string literal by %q rather than concatenated: the command itself stays
	// compiled in.
	command := fmt.Sprintf(createCheckpoint, label)

	raw, err := m.run(ctx, psExe, "-NoProfile", "-NonInteractive", "-Command", command)
	if err != nil {
		return Point{}, fmt.Errorf("restore: create system restore point: %w", err)
	}

	return Point{
		Kind:      "Windows System Restore",
		Reference: sequenceNumber(raw),
		Label:     label,
		Created:   time.Now().UTC(),
	}, nil
}
