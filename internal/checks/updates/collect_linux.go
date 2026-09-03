package updates

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/ZanOzair/supportone/internal/platform"
)

const (
	aptGetExe = "apt-get"
	dnfExe    = "dnf"

	// dpkgStatus and rpmDB change whenever a package is installed or removed,
	// which is the closest thing Linux keeps to "when did this machine last
	// take updates".
	dpkgStatus = "/var/lib/dpkg/status"
	rpmDB      = "/var/lib/rpm/rpmdb.sqlite"
)

func collectUpdates(ctx context.Context, run platform.Runner) (updateFacts, error) {
	facts := updateFacts{Pending: -1}

	// -s simulates and Debug::NoLocking lets it run unprivileged; neither
	// touches the network, so a pending count reflects the local cache.
	if out, err := run(ctx, aptGetExe, "-s", "-o", "Debug::NoLocking=true", "upgrade"); err == nil {
		facts.Pending = countAptUpgrades(out)
		facts.Source = "apt package cache"
		facts.LastInstalled = fileModTime(dpkgStatus)
		return facts, nil
	} else if !errors.Is(err, platform.ErrToolMissing) {
		return facts, err
	}

	// -C keeps dnf to its cache. It exits 100 when updates are available,
	// which is a finding rather than a failure, so output is parsed either way.
	out, err := run(ctx, dnfExe, "-C", "check-update")
	if errors.Is(err, platform.ErrToolMissing) {
		return facts, err
	}
	if len(out) > 0 {
		facts.Pending = countDnfUpdates(out)
		facts.Source = "dnf package cache"
		facts.LastInstalled = fileModTime(rpmDB)
		return facts, nil
	}
	return facts, err
}

// fileModTime returns a file's modification time, or the zero time when it
// cannot be read. Zero means "not reported" everywhere in this package.
func fileModTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime().UTC()
}
