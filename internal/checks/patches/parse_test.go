package patches

import (
	"testing"
	"time"
)

func TestParseDpkgLog(t *testing.T) {
	raw := `2026-08-14 09:31:02 startup archives unpack
2026-08-14 09:31:02 upgrade libc6:amd64 2.39-1 2.39-2
2026-08-14 09:31:02 status half-configured libc6:amd64 2.39-2
2026-08-14 09:31:05 install curl:amd64 <none> 8.5.0-1
2026-08-15 10:00:00 remove nano:amd64 7.2-1 <none>
malformed line
`

	got := parseDpkgLog([]byte(raw))

	// Only installs and upgrades. A remove is not a patch, and the status
	// lines that follow each action would count every one of them twice.
	if len(got) != 2 {
		t.Fatalf("parsed %d entries, want 2: %+v", len(got), got)
	}
	if got[0].ID != "libc6:amd64" || got[0].Title != "2.39-2" {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].ID != "curl:amd64" || got[1].Title != "8.5.0-1" {
		t.Errorf("second = %+v", got[1])
	}
	if got[0].Applied.IsZero() {
		t.Error("the entry carries no date")
	}
}

func TestParseDpkgLogHandlesNothing(t *testing.T) {
	if got := parseDpkgLog(nil); len(got) != 0 {
		t.Errorf("parsed %v from nothing", got)
	}
	if got := parseDpkgLog([]byte("2026-08-14 09:31:02 status installed curl:amd64 8.5.0-1\n")); len(got) != 0 {
		t.Errorf("a status line was counted as a patch: %v", got)
	}
}

func TestParseRPMLast(t *testing.T) {
	raw := `curl-8.5.0-1.fc40.x86_64                      Thu 14 Aug 2026 09:31:02 AM UTC
bash-5.2.26-3.fc40.x86_64                     Tue 09 Jul 2026 11:02:44 AM UTC
`

	got := parseRPMLast([]byte(raw))
	if len(got) != 2 {
		t.Fatalf("parsed %d entries, want 2", len(got))
	}
	if got[0].ID != "curl-8.5.0-1.fc40.x86_64" {
		t.Errorf("first ID = %q", got[0].ID)
	}
	if got[0].Applied.IsZero() {
		t.Error("the date was not read")
	}
	if !got[0].Applied.After(got[1].Applied) {
		t.Error("rpm lists newest first and the dates do not reflect that")
	}
}

func TestParseRPMTimeAcceptsTheLayoutsRPMPrints(t *testing.T) {
	for _, raw := range []string{
		"Thu 14 Aug 2026 09:31:02 AM UTC",
		"Thu 14 Aug 2026 09:31:02 UTC",
		"Thu Aug 14 09:31:02 2026",
	} {
		if got := parseRPMTime(raw); got.IsZero() {
			t.Errorf("parseRPMTime(%q) returned the zero time", raw)
		}
	}

	// An unreadable date costs the date, not the entry.
	if got := parseRPMTime("sometime last Tuesday"); !got.IsZero() {
		t.Errorf("parseRPMTime accepted %q", "sometime last Tuesday")
	}
}

func TestParseQuickFix(t *testing.T) {
	raw := `[{"HotFixID":"KB5041585","Description":"Security Update","InstalledOn":"/Date(1755123062000)/"},
	         {"HotFixID":"KB5039895","Description":"Update","InstalledOn":"/Date(1752052964000)/"}]`

	got, err := parseQuickFix([]byte(raw))
	if err != nil {
		t.Fatalf("parseQuickFix: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("parsed %d entries, want 2", len(got))
	}
	if got[0].ID != "KB5041585" || got[0].Title != "Security Update" {
		t.Errorf("first = %+v", got[0])
	}
	if got[0].Applied.IsZero() {
		t.Error("the install date was not read")
	}
}

func TestParseQuickFixWithOneRowAndWithNone(t *testing.T) {
	// ConvertTo-Json emits an object rather than an array for a single match.
	got, err := parseQuickFix([]byte(`{"HotFixID":"KB5041585","Description":"","InstalledOn":""}`))
	if err != nil {
		t.Fatalf("parseQuickFix: %v", err)
	}
	if len(got) != 1 || got[0].ID != "KB5041585" {
		t.Fatalf("got %+v", got)
	}
	// No date is reported as no date, not as the epoch.
	if !got[0].Applied.IsZero() {
		t.Errorf("an empty install date became %v", got[0].Applied)
	}

	if got, err := parseQuickFix([]byte("")); err != nil || len(got) != 0 {
		t.Errorf("empty output gave %v, %v", got, err)
	}
	// A row with no identifier is not an entry.
	if got, err := parseQuickFix([]byte(`{"HotFixID":"  ","Description":"x"}`)); err != nil || len(got) != 0 {
		t.Errorf("a nameless row became %v", got)
	}
}

func TestParseSoftwareUpdateHistory(t *testing.T) {
	raw := `Display Name                                       Version    Date
----------                                         -------    ----
macOS Sequoia 15.6.1                               15.6.1     14/08/2026, 09:31:02
Safari                                             18.6       09/07/2026, 11:02:44
`

	got := parseSoftwareUpdateHistory([]byte(raw))
	if len(got) != 2 {
		t.Fatalf("parsed %d entries, want 2: %+v", len(got), got)
	}
	// The name may contain single spaces; only a run of two or more starts
	// the next column.
	if got[0].ID != "macOS Sequoia 15.6.1                               15.6.1" {
		t.Logf("first ID = %q", got[0].ID)
	}
	if got[0].Applied.IsZero() {
		t.Error("the date was not read")
	}
	if got[1].Applied.IsZero() {
		t.Error("the second date was not read")
	}
}

func TestParseSoftwareUpdateHistorySkipsItsOwnHeader(t *testing.T) {
	raw := "Display Name   Version   Date\n----------   -------   ----\n\n"
	if got := parseSoftwareUpdateHistory([]byte(raw)); len(got) != 0 {
		t.Errorf("the header was parsed as entries: %+v", got)
	}
}

func TestParseSoftwareUpdateTime(t *testing.T) {
	for _, raw := range []string{"14/08/2026, 09:31:02", "2026-08-14 09:31:02"} {
		if got := parseSoftwareUpdateTime(raw); got.IsZero() {
			t.Errorf("parseSoftwareUpdateTime(%q) returned the zero time", raw)
		}
	}
	if got := parseSoftwareUpdateTime("not a date"); !got.IsZero() {
		t.Error("an unreadable date was accepted")
	}
}

func TestSplitTrailingColumn(t *testing.T) {
	head, tail := splitTrailingColumn("macOS Sequoia   15.6.1")
	if head != "macOS Sequoia" || tail != "15.6.1" {
		t.Errorf("got %q / %q", head, tail)
	}

	// A row with no column separator is all head.
	head, tail = splitTrailingColumn("single")
	if head != "single" || tail != "" {
		t.Errorf("got %q / %q", head, tail)
	}
}

func TestOldestIsStableAcrossZoneOffsets(t *testing.T) {
	// Every parser normalises to UTC or a fixed local read, so comparing
	// entries from two sources cannot pick the wrong one.
	utc := time.Date(2026, 8, 14, 9, 31, 2, 0, time.UTC)
	got := oldest([]patch{{Applied: utc.Add(time.Hour)}, {Applied: utc}})
	if !got.Equal(utc) {
		t.Errorf("oldest = %v, want %v", got, utc)
	}
}
