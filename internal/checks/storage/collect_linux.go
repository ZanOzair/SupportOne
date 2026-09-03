package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"golang.org/x/sys/unix"

	"github.com/ZanOzair/supportone/internal/platform"
)

// Roots are variables so tests can point them at recorded trees.
var (
	procRoot = "/proc"
	sysRoot  = "/sys"
)

const smartctlExe = "smartctl"

func collectVolumes(_ context.Context, _ platform.Runner) ([]volume, error) {
	data, err := os.ReadFile(filepath.Join(procRoot, "self/mounts"))
	if err != nil {
		return nil, fmt.Errorf("storage: read mount table: %w", err)
	}

	seen := make(map[string]bool)
	var out []volume
	for _, entry := range parseMounts(data) {
		if !isRealFilesystem(entry) || seen[entry.Device] {
			continue
		}

		var st unix.Statfs_t
		if err := unix.Statfs(entry.Mount, &st); err != nil {
			// A mount the agent cannot stat (a stale network share, a
			// permission-restricted path) is skipped rather than reported
			// with invented numbers.
			continue
		}
		if st.Blocks == 0 {
			continue
		}

		seen[entry.Device] = true
		out = append(out, volume{
			Mount:      entry.Mount,
			Device:     entry.Device,
			Filesystem: entry.FSType,
			TotalBytes: st.Blocks * uint64(st.Bsize),
			FreeBytes:  st.Bavail * uint64(st.Bsize),
		})
	}
	return out, nil
}

func collectDisks(ctx context.Context, run platform.Runner) ([]disk, error) {
	entries, err := os.ReadDir(filepath.Join(sysRoot, "block"))
	if err != nil {
		return nil, fmt.Errorf("storage: list block devices: %w", err)
	}

	var names []string
	for _, entry := range entries {
		if blockDeviceIsPhysical(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	var out []disk
	for _, name := range names {
		device := filepath.Join("/dev", name)

		// smartctl exits non-zero when a drive reports problems, so its output
		// is parsed before its exit status is judged.
		raw, runErr := run(ctx, smartctlExe, "--json", "-H", "-A", device)
		if len(raw) == 0 {
			if runErr != nil {
				return nil, runErr
			}
			continue
		}

		d, parseErr := parseSmartctl(raw, device)
		if parseErr != nil {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}
