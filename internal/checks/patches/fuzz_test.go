package patches

import "testing"

// Package histories, in four formats, none of which this project controls.

func FuzzParseDpkgLog(f *testing.F) {
	f.Add([]byte("2026-01-01 10:00:00 upgrade openssl:amd64 3.0.1 3.0.2\n"))
	f.Add([]byte("2026-01-01 upgrade"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, data []byte) {
		_ = parseDpkgLog(data)
		_ = parseRPMLast(data)
		_ = parseSoftwareUpdateHistory(data)
	})
}

func FuzzParseQuickFix(f *testing.F) {
	f.Add([]byte(`[{"HotFixID":"KB5034123","InstalledOn":"/Date(1700000000000)/"}]`))
	f.Add([]byte(`[{"HotFixID":`))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseQuickFix(data)
	})
}

func FuzzParsePatchTimes(f *testing.F) {
	f.Add("Mon 01 Jan 2026 10:00:00 AM UTC")
	f.Add("2026-01-01T10:00:00Z")
	f.Add("")

	f.Fuzz(func(t *testing.T, s string) {
		_ = parseRPMTime(s)
		_ = parseSoftwareUpdateTime(s)
	})
}
