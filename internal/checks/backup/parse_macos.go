package backup

import (
	"strings"
	"time"
)

// parseDestinationInfo reads `tmutil destinationinfo`, which prints one block
// per destination:
//
//	Name          : Backup Drive
//	Kind          : Local
//	Mount Point   : /Volumes/Backup Drive
//	ID            : 0C2B...
//
// and prints "No destinations configured." when there are none.
func parseDestinationInfo(raw []byte) (configured bool, destination string) {
	text := string(raw)
	if strings.Contains(text, "No destinations configured") {
		return false, ""
	}

	for _, line := range strings.Split(text, "\n") {
		name, rest, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		if strings.TrimSpace(name) == "Name" {
			// The destination's name, not its mount point: a path under
			// /Volumes can carry a person's name, and this is shown to
			// whoever the report is sent to.
			return true, strings.TrimSpace(rest)
		}
	}

	// Something was printed that is not the "none configured" message, so a
	// destination exists even though its name was not readable.
	if strings.TrimSpace(text) != "" {
		return true, ""
	}
	return false, ""
}

// parseLatestBackup reads the path `tmutil latestbackup` prints, whose last
// element is a timestamp: "/Volumes/Backup/.../2026-09-01-134500".
func parseLatestBackup(raw []byte) time.Time {
	path := strings.TrimSpace(string(raw))
	if path == "" {
		return time.Time{}
	}

	name := path
	if at := strings.LastIndex(path, "/"); at >= 0 {
		name = path[at+1:]
	}
	// Time Machine names snapshots in local time and records no zone, so it
	// is read as local: an hour of error either way does not change a verdict
	// measured in days.
	stamp, err := time.ParseInLocation("2006-01-02-150405", name, time.Local)
	if err != nil {
		return time.Time{}
	}
	return stamp
}
