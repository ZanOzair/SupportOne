package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"golang.org/x/sys/unix"

	"github.com/ZanOzair/SupportOne/internal/platform"
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

		blockSize := nonNegative(st.Bsize)
		seen[entry.Device] = true
		out = append(out, volume{
			Mount:      entry.Mount,
			Device:     entry.Device,
			Filesystem: entry.FSType,
			TotalBytes: st.Blocks * blockSize,
			FreeBytes:  st.Bavail * blockSize,
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

// nonNegative converts a signed block size from the kernel. A negative value
// cannot happen, and turning one into a huge unsigned number would report a
// fictional drive size, so it becomes zero and the volume reads as unmeasured.
// It is generic over both widths because Statfs_t.Bsize is int64 on a 64-bit
// kernel and int32 on a 32-bit one. Converting at the call site would compile
// on both but read as a redundant cast on the machine most people build on,
// and 32-bit is exactly where a disk-space check earns its keep.
func nonNegative[T int32 | int64](v T) uint64 {
	if v < 0 {
		return 0
	}
	return uint64(v)
}
