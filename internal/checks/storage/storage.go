// Package storage reports on disks: how full they are, and whether they are
// failing.
//
// Both checks are read-only. Neither writes to a disk, mounts anything, or
// changes a partition table.
package storage

import (
	"context"
	"fmt"
	"sort"

	"github.com/ZanOzair/supportone/internal/checks"
	"github.com/ZanOzair/supportone/internal/platform"
)

// volume is one mounted filesystem the user would recognise as a drive.
type volume struct {
	Mount      string `json:"mount"`
	Device     string `json:"device,omitempty"`
	Filesystem string `json:"filesystem,omitempty"`
	TotalBytes uint64 `json:"total_bytes"`
	FreeBytes  uint64 `json:"free_bytes"`
}

// FreePercent returns how much of the volume is unused.
func (v volume) FreePercent() float64 {
	if v.TotalBytes == 0 {
		return 0
	}
	return float64(v.FreeBytes) / float64(v.TotalBytes) * 100
}

// disk is one physical drive and the health verdict its OS reports.
type disk struct {
	Name   string `json:"name"`
	Model  string `json:"model,omitempty"`
	Status status `json:"status"`

	// Reallocated is the count of sectors the drive has retired because they
	// went bad. Absent where the platform does not expose SMART attributes.
	Reallocated *int `json:"reallocated_sectors,omitempty"`
}

// status is what the platform says about a drive's health.
type status string

const (
	statusHealthy status = "healthy"
	statusFailing status = "failing"
	statusUnknown status = "unknown"
)

// Thresholds for disk.volumes. They are stated here and documented in
// docs/CHECKS.md so nothing about the verdict is a mystery.
const (
	lowSpacePercent      = 10.0
	criticalSpacePercent = 5.0
)

// Message keys for this package's results.
const (
	keyVolumesOK       = "check.disk.volumes.ok"
	keyVolumesLow      = "check.disk.volumes.low"
	keyVolumesCritical = "check.disk.volumes.critical"
	keyVolumesNone     = "check.disk.volumes.none"

	keySMARTOK       = "check.disk.smart.ok"
	keySMARTFailing  = "check.disk.smart.failing"
	keySMARTBadSpots = "check.disk.smart.bad_spots"
	keySMARTUnknown  = "check.disk.smart.unknown"
	keySMARTNoDisks  = "check.disk.smart.no_disks"
)

type volumesCheck struct{ run platform.Runner }

func (volumesCheck) ID() string               { return "disk.volumes" }
func (volumesCheck) Platforms() []platform.OS { return platform.All() }
func (volumesCheck) RequiresAdmin() bool      { return false }

func (c volumesCheck) Run(ctx context.Context) (checks.Result, error) {
	volumes, err := collectVolumes(ctx, c.run)
	if err != nil {
		return checks.UnknownFor(err), nil
	}
	if len(volumes) == 0 {
		return checks.Unknown(keyVolumesNone), nil
	}

	sort.Slice(volumes, func(i, j int) bool { return volumes[i].FreePercent() < volumes[j].FreePercent() })
	detail := map[string]any{"volumes": volumes}

	tightest := volumes[0]
	free := checks.HumanBytes(tightest.FreeBytes)
	percent := fmt.Sprintf("%.0f", tightest.FreePercent())

	switch {
	case tightest.FreePercent() < criticalSpacePercent:
		return checks.Urgent(keyVolumesCritical, tightest.Mount, free, percent).With(detail), nil
	case tightest.FreePercent() < lowSpacePercent:
		return checks.Attention(keyVolumesLow, tightest.Mount, free, percent).With(detail), nil
	default:
		return checks.OK(keyVolumesOK, len(volumes), tightest.Mount, free).With(detail), nil
	}
}

type smartCheck struct{ run platform.Runner }

func (smartCheck) ID() string               { return "disk.smart" }
func (smartCheck) Platforms() []platform.OS { return platform.All() }

// RequiresAdmin is platform-dependent: reading SMART attributes on Linux needs
// root, while Windows and macOS expose a health verdict to any user. Claiming
// otherwise would either skip the check where it would have worked or run it
// where it can only fail.
func (smartCheck) RequiresAdmin() bool { return platform.Current() == platform.Linux }

func (c smartCheck) Run(ctx context.Context) (checks.Result, error) {
	disks, err := collectDisks(ctx, c.run)
	if err != nil {
		return checks.UnknownFor(err), nil
	}
	if len(disks) == 0 {
		return checks.Unknown(keySMARTNoDisks), nil
	}

	detail := map[string]any{"disks": disks}
	var failing, unknown int
	var worstReallocated int
	var worstName string

	for _, d := range disks {
		switch d.Status {
		case statusFailing:
			failing++
			if worstName == "" {
				worstName = d.Name
			}
		case statusUnknown:
			unknown++
		case statusHealthy:
		}
		if d.Reallocated != nil && *d.Reallocated > worstReallocated {
			worstReallocated = *d.Reallocated
			if failing == 0 {
				worstName = d.Name
			}
		}
	}

	switch {
	case failing > 0:
		return checks.Urgent(keySMARTFailing, worstName).With(detail), nil
	case worstReallocated > 0:
		// Retired sectors are not yet a failure verdict, but they are the
		// first thing a failing drive does. Say so plainly.
		return checks.Attention(keySMARTBadSpots, worstName, worstReallocated).With(detail), nil
	case unknown == len(disks):
		return checks.Unknown(keySMARTUnknown).With(detail), nil
	default:
		return checks.OK(keySMARTOK, len(disks)).With(detail), nil
	}
}

func init() {
	checks.MustRegister(volumesCheck{run: platform.RunRead})
	checks.MustRegister(smartCheck{run: platform.RunRead})
}
