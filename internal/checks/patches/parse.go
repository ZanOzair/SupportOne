package patches

import (
	"bufio"
	"bytes"
	"strings"
	"time"

	"github.com/ZanOzair/SupportOne/internal/checks/cim"
)

// The parsers below read each platform's own record. None carries a build
// constraint, so Windows and macOS output is testable from recorded fixtures
// on any machine.

// oldest returns the earliest dated entry, which is how far back the record
// reaches. It is zero when nothing carries a date, which is reported as
// unknown rather than as "the beginning of time".
func oldest(applied []patch) time.Time {
	var earliest time.Time
	for _, p := range applied {
		if p.Applied.IsZero() {
			continue
		}
		if earliest.IsZero() || p.Applied.Before(earliest) {
			earliest = p.Applied
		}
	}
	return earliest
}

// parseDpkgLog reads dpkg's log, whose install and upgrade lines look like:
//
//	2026-08-14 09:31:02 upgrade libc6:amd64 2.39-1 2.39-2
//	2026-08-14 09:31:05 install curl:amd64 <none> 8.5.0-1
//
// Only "install" and "upgrade" are read. A "remove" is not a patch, and the
// "status" lines that follow each action would count every one of them twice.
func parseDpkgLog(data []byte) []patch {
	var out []patch

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 5 {
			continue
		}
		action := fields[2]
		if action != "install" && action != "upgrade" {
			continue
		}

		applied, err := time.ParseInLocation("2006-01-02 15:04:05", fields[0]+" "+fields[1], time.Local)
		if err != nil {
			continue
		}

		name := fields[3]
		version := fields[len(fields)-1]
		out = append(out, patch{ID: name, Title: version, Applied: applied})
	}
	return out
}

// parseRPMLast reads `rpm -qa --last`, whose lines are a package name padded
// out to a column and then a date:
//
//	curl-8.5.0-1.fc40.x86_64                      Thu 14 Aug 2026 09:31:02 AM UTC
func parseRPMLast(data []byte) []patch {
	var out []patch

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		name := fields[0]
		rest := strings.TrimSpace(strings.TrimPrefix(line, name))

		out = append(out, patch{ID: name, Applied: parseRPMTime(rest)})
	}
	return out
}

// rpmLayouts are the shapes rpm prints a date in, which vary by locale and by
// whether the build was compiled with a 12-hour clock.
var rpmLayouts = []string{
	"Mon 02 Jan 2006 03:04:05 PM MST",
	"Mon 02 Jan 2006 15:04:05 MST",
	"Mon Jan 02 15:04:05 2006",
}

// parseRPMTime reads whichever of those layouts matches, and returns the zero
// time when none does — an unreadable date costs the date, not the entry.
func parseRPMTime(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range rpmLayouts {
		if stamp, err := time.Parse(layout, s); err == nil {
			return stamp.UTC()
		}
	}
	return time.Time{}
}

// windowsPatch mirrors one row of the Win32_QuickFixEngineering query.
type windowsPatch struct {
	HotFixID    string `json:"HotFixID"`
	Description string `json:"Description"`
	InstalledOn string `json:"InstalledOn"`
}

// parseQuickFix reads the CIM response listing installed hotfixes.
func parseQuickFix(raw []byte) ([]patch, error) {
	rows, err := cim.Unmarshal[windowsPatch](raw)
	if err != nil {
		return nil, err
	}

	out := make([]patch, 0, len(rows))
	for _, row := range rows {
		id := strings.TrimSpace(row.HotFixID)
		if id == "" {
			continue
		}
		out = append(out, patch{
			ID:      id,
			Title:   strings.TrimSpace(row.Description),
			Applied: cim.ParseTime(row.InstalledOn),
		})
	}
	return out, nil
}

// parseSoftwareUpdateHistory reads `softwareupdate --history`, whose table is:
//
//	Display Name                        Version    Date
//	----------                          -------    ----
//	macOS Sequoia 15.6.1                15.6.1     14/08/2026, 09:31:02
func parseSoftwareUpdateHistory(data []byte) []patch {
	var out []patch

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), " \t")
		trimmed := strings.TrimSpace(line)

		// Skip the header, the rule under it, and blank lines.
		if trimmed == "" || strings.HasPrefix(trimmed, "Display Name") || strings.HasPrefix(trimmed, "---") {
			continue
		}

		// The date is the tail after the last run of two or more spaces; the
		// name may itself contain single spaces.
		name, stamp := splitTrailingColumn(line)
		if name == "" {
			continue
		}
		out = append(out, patch{ID: name, Applied: parseSoftwareUpdateTime(stamp)})
	}
	return out
}

// splitTrailingColumn splits a fixed-width row into its leading text and its
// last column, which are separated by two or more spaces.
func splitTrailingColumn(line string) (head, tail string) {
	at := strings.LastIndex(line, "  ")
	if at < 0 {
		return strings.TrimSpace(line), ""
	}
	return strings.TrimSpace(line[:at]), strings.TrimSpace(line[at:])
}

// softwareUpdateLayouts are the shapes macOS prints the install date in, which
// follow the machine's locale.
var softwareUpdateLayouts = []string{
	"02/01/2006, 15:04:05",
	"01/02/2006, 15:04:05",
	"2006-01-02 15:04:05",
}

func parseSoftwareUpdateTime(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range softwareUpdateLayouts {
		if stamp, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return stamp.UTC()
		}
	}
	return time.Time{}
}
