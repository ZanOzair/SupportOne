package backup

import (
	"testing"
	"time"
)

func TestParseDestinationInfo(t *testing.T) {
	configured := `====================================================
Name          : Backup Drive
Kind          : Local
Mount Point   : /Volumes/Backup Drive
ID            : 0C2B1F3A-1111-2222-3333-444455556666
`

	ok, name := parseDestinationInfo([]byte(configured))
	if !ok {
		t.Error("a configured destination was read as none")
	}
	// The name, not the mount point: a path under /Volumes can carry a
	// person's name, and this goes into a report.
	if name != "Backup Drive" {
		t.Errorf("destination = %q, want %q", name, "Backup Drive")
	}

	ok, name = parseDestinationInfo([]byte("tmutil: No destinations configured.\n"))
	if ok || name != "" {
		t.Errorf("got %v/%q, want no destination", ok, name)
	}

	ok, _ = parseDestinationInfo([]byte(""))
	if ok {
		t.Error("empty output was read as a configured destination")
	}

	// Output that is neither the "none" message nor parseable still means a
	// destination exists.
	if ok, _ = parseDestinationInfo([]byte("something unexpected\n")); !ok {
		t.Error("unrecognised output was read as no destination at all")
	}
}

func TestParseLatestBackup(t *testing.T) {
	got := parseLatestBackup([]byte("/Volumes/Backup/Backups.backupdb/mac/2026-09-01-134500\n"))

	want := time.Date(2026, 9, 1, 13, 45, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("parseLatestBackup = %v, want %v", got, want)
	}

	for _, raw := range []string{"", "\n", "/Volumes/Backup/not-a-date", "no-slashes-either"} {
		if got := parseLatestBackup([]byte(raw)); !got.IsZero() {
			t.Errorf("parseLatestBackup(%q) = %v, want the zero time", raw, got)
		}
	}
}

func TestParseShadowCopies(t *testing.T) {
	raw := `{"Configured":true,"InstallDate":"/Date(1756900000000)/","Volume":"C:\\"}`

	got, err := parseShadowCopies([]byte(raw))
	if err != nil {
		t.Fatalf("parseShadowCopies: %v", err)
	}
	if !got.Supported || !got.Configured {
		t.Errorf("got %+v, want a configured, supported mechanism", got)
	}
	if got.Last.IsZero() {
		t.Error("the backup date was not read")
	}
	if got.Destination != "C:" {
		t.Errorf("destination = %q, want %q", got.Destination, "C:")
	}
}

func TestParseShadowCopiesWithNothingConfigured(t *testing.T) {
	got, err := parseShadowCopies([]byte(`{"Configured":false,"InstallDate":null,"Volume":null}`))
	if err != nil {
		t.Fatalf("parseShadowCopies: %v", err)
	}
	// Nothing found is still a platform we looked at.
	if !got.Supported {
		t.Error("Supported = false though the query ran")
	}
	if got.Configured {
		t.Error("Configured = true with no shadow copy")
	}
}

func TestParseShadowCopiesWithAnEmptyResponse(t *testing.T) {
	got, err := parseShadowCopies([]byte(""))
	if err != nil {
		t.Fatalf("parseShadowCopies: %v", err)
	}
	if !got.Supported || got.Configured {
		t.Errorf("got %+v, want supported with nothing configured", got)
	}
}

func TestDriveLetterDropsAVolumeGUID(t *testing.T) {
	cases := map[string]string{
		`C:\`:                        "C:",
		"D:":                         "D:",
		`\\?\Volume{1a2b3c4d-0000}\`: "",
		"":                           "",
		"   ":                        "",
	}

	for volume, want := range cases {
		if got := driveLetter(volume); got != want {
			t.Errorf("driveLetter(%q) = %q, want %q", volume, got, want)
		}
	}
}
