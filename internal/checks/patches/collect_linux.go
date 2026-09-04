package patches

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

// logRoot is a variable so tests can point it at a recorded /var/log tree.
var logRoot = "/var/log"

const rpmExe = "rpm"

// collectPatches reads the package manager's own record.
//
// Debian's dpkg log and rpm's install database are what a Linux machine keeps,
// and both are readable without elevation. Neither goes back forever: dpkg logs
// are rotated on the distribution's schedule, which is why the horizon is
// reported alongside the entries.
func collectPatches(ctx context.Context, run platform.Runner) (patchFacts, error) {
	if facts, ok := collectDpkg(); ok {
		return facts, nil
	}

	// -qa --last lists installed packages newest first, from the local
	// database. It makes no network request.
	out, err := run(ctx, rpmExe, "-qa", "--last")
	if err != nil {
		return patchFacts{}, fmt.Errorf("patches: read the package record: %w", err)
	}

	applied := parseRPMLast(out)
	return patchFacts{Source: "rpm database", Applied: applied, Horizon: oldest(applied)}, nil
}

// collectDpkg reads every dpkg log the machine still has, rotated ones
// included, so the horizon reflects what is actually on disk.
func collectDpkg() (patchFacts, bool) {
	names, err := filepath.Glob(filepath.Join(logRoot, "dpkg.log*"))
	if err != nil || len(names) == 0 {
		return patchFacts{}, false
	}

	var applied []patch
	for _, name := range names {
		// #nosec G304 -- the path comes from a glob of a fixed directory, not
		// from user input.
		data, err := os.ReadFile(name)
		if err != nil {
			// A rotated log compressed by logrotate is skipped rather than
			// decompressed: the uncompressed ones cover the recent period a
			// report is about.
			continue
		}
		applied = append(applied, parseDpkgLog(data)...)
	}
	if len(applied) == 0 {
		// The logs exist but record no installs. That is a real answer.
		return patchFacts{Source: "dpkg log"}, true
	}
	return patchFacts{Source: "dpkg log", Applied: applied, Horizon: oldest(applied)}, true
}
