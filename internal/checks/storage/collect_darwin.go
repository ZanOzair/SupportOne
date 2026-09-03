package storage

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

const diskutilExe = "diskutil"

func collectVolumes(_ context.Context, _ platform.Runner) ([]volume, error) {
	count, err := unix.Getfsstat(nil, unix.MNT_NOWAIT)
	if err != nil {
		return nil, fmt.Errorf("storage: count mounted filesystems: %w", err)
	}

	stats := make([]unix.Statfs_t, count)
	if _, err := unix.Getfsstat(stats, unix.MNT_NOWAIT); err != nil {
		return nil, fmt.Errorf("storage: read mounted filesystems: %w", err)
	}

	var out []volume
	for _, st := range stats {
		if st.Blocks == 0 {
			continue
		}
		fstype := bytesToString(st.Fstypename[:])
		// devfs, autofs and the read-only system snapshot are plumbing, not
		// drives the user has.
		if fstype == "devfs" || fstype == "autofs" {
			continue
		}

		out = append(out, volume{
			Mount:      bytesToString(st.Mntonname[:]),
			Device:     bytesToString(st.Mntfromname[:]),
			Filesystem: fstype,
			TotalBytes: st.Blocks * uint64(st.Bsize),
			FreeBytes:  st.Bavail * uint64(st.Bsize),
		})
	}
	return out, nil
}

func collectDisks(ctx context.Context, run platform.Runner) ([]disk, error) {
	out, err := run(ctx, diskutilExe, "info", "-all")
	if err != nil {
		return nil, err
	}
	return parseDiskutilInfo(out), nil
}

// bytesToString reads the NUL-terminated character arrays the darwin statfs
// structure uses for names.
func bytesToString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return strings.TrimSpace(string(b))
}
