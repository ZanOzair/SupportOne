package events

import (
	"strings"
	"testing"
)

func TestParseWindowsEvents(t *testing.T) {
	fixture := []byte(`[{"TimeCreated":"/Date(1725282000000)/","Id":7000,"ProviderName":"Service Control Manager",` +
		`"LevelDisplayName":"Error","Message":"The Print Spooler service failed to start."},` +
		`{"TimeCreated":"/Date(1725285600000)/","Id":41,"ProviderName":"Kernel-Power",` +
		`"LevelDisplayName":"Critical","Message":"The system rebooted without cleanly shutting down first."}]`)

	events, err := parseWindowsEvents(fixture)
	if err != nil {
		t.Fatalf("parseWindowsEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].Source != "Service Control Manager" || events[0].ID != "7000" || events[0].Level != levelError {
		t.Errorf("event = %+v", events[0])
	}
	if events[1].Level != levelCritical {
		t.Errorf("level = %q, want critical", events[1].Level)
	}
	if events[0].Time.IsZero() {
		t.Error("time was not parsed")
	}
}

func TestParseJournal(t *testing.T) {
	fixture := []byte(`{"__REALTIME_TIMESTAMP":"1725282000000000","SYSLOG_IDENTIFIER":"kernel","PRIORITY":"3","MESSAGE":"ata1: link is slow"}
not json at all
{"__REALTIME_TIMESTAMP":"1725282600000000","_SYSTEMD_UNIT":"cups.service","PRIORITY":"2","MESSAGE":"printer offline"}
{"__REALTIME_TIMESTAMP":"bad","_COMM":"gdm","PRIORITY":"not a number","MESSAGE":"session failed"}
`)

	events := parseJournal(fixture)
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3 (the malformed line is skipped, not fatal): %+v", len(events), events)
	}
	if events[0].Source != "kernel" || events[0].Level != levelError {
		t.Errorf("event = %+v", events[0])
	}
	if events[1].Source != "cups.service" || events[1].Level != levelCritical {
		t.Errorf("priority 2 should be critical: %+v", events[1])
	}
	if !events[2].Time.IsZero() {
		t.Errorf("unparseable timestamp = %v, want zero", events[2].Time)
	}
	if events[2].Level != levelError {
		t.Errorf("unparseable priority = %q, want error", events[2].Level)
	}
}

func TestParseMacLog(t *testing.T) {
	fixture := []byte(`{"timestamp":"2026-09-02 13:20:00.123456+0000","subsystem":"com.apple.TimeMachine",` +
		`"processImagePath":"/usr/libexec/backupd","eventMessage":"Backup failed with error 11","messageType":"Error"}
{"timestamp":"2026-09-02 13:25:00.000000+0000","processImagePath":"/usr/sbin/cupsd",` +
		`"eventMessage":"printer not responding","messageType":"Fault"}
[
`)

	events := parseMacLog(fixture)
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2: %+v", len(events), events)
	}
	if events[0].Source != "com.apple.TimeMachine" || events[0].Level != levelError {
		t.Errorf("event = %+v", events[0])
	}
	if events[1].Source != "cupsd" {
		t.Errorf("source = %q, want the process basename when there is no subsystem", events[1].Source)
	}
	if events[1].Level != levelCritical {
		t.Errorf("a fault should be critical: %+v", events[1])
	}
}

func TestFindRepetitions(t *testing.T) {
	var events []logEvent
	for i := 0; i < 12; i++ {
		events = append(events, logEvent{Source: "disk", ID: "153", Level: levelError})
	}
	for i := 0; i < 3; i++ {
		events = append(events, logEvent{Source: "bluetooth", ID: "17", Level: levelError})
	}

	repeats := findRepetitions(events)
	if len(repeats) != 1 {
		t.Fatalf("repeats = %+v, want only the source above the threshold", repeats)
	}
	if repeats[0].Source != "disk" || repeats[0].Count != 12 {
		t.Errorf("repeat = %+v", repeats[0])
	}
}

func TestCountCritical(t *testing.T) {
	events := []logEvent{
		{Level: levelError},
		{Level: levelCritical},
		{Level: levelCritical},
	}
	if got := countCritical(events); got != 2 {
		t.Errorf("countCritical = %d, want 2", got)
	}
}

func TestTruncateKeepsMessagesOnOneLine(t *testing.T) {
	got := truncate("first line\nsecond line")
	if strings.Contains(got, "\n") {
		t.Errorf("truncate left a newline in %q", got)
	}

	long := strings.Repeat("x", maxMessageLength+50)
	if got := truncate(long); len([]rune(got)) != maxMessageLength+1 {
		t.Errorf("truncated length = %d runes, want %d plus the ellipsis", len([]rune(got)), maxMessageLength)
	}
}
