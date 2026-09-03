package restore

import "strings"

// The two platform makers each read one identifier out of one line of tool
// output. Both parsers live here, with no build constraint, so the output of a
// Windows or macOS tool can be tested on any machine — the same split the
// diagnostic checks use.

// sequenceNumber pulls a checkpoint's identifier out of PowerShell's response,
// so a technician can find the restore point later. An unreadable response
// costs the reference, not the restore point.
func sequenceNumber(raw []byte) string {
	const key = `"SequenceNumber":`
	s := string(raw)
	at := strings.Index(s, key)
	if at < 0 {
		return ""
	}
	rest := s[at+len(key):]
	end := strings.IndexAny(rest, ",}")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

// snapshotName reads the snapshot identifier out of tmutil's confirmation
// line: "Created local snapshot with date: 2026-09-03-134500".
func snapshotName(raw []byte) string {
	const key = "date: "
	s := strings.TrimSpace(string(raw))
	at := strings.Index(s, key)
	if at < 0 {
		return ""
	}
	return strings.TrimSpace(s[at+len(key):])
}
