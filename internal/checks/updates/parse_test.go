package updates

import (
	"testing"
	"time"

	"github.com/ZanOzair/supportone/internal/checks"
)

func TestCountAptUpgrades(t *testing.T) {
	fixture := []byte(`NOTE: This is only a simulation!
Reading package lists...
The following packages will be upgraded:
  curl libcurl4 openssl
3 upgraded, 0 newly installed, 0 to remove and 0 not upgraded.
Inst curl [8.5.0-2ubuntu1] (8.5.0-2ubuntu2 Ubuntu:24.04/noble-updates [amd64])
Inst libcurl4 [8.5.0-2ubuntu1] (8.5.0-2ubuntu2 Ubuntu:24.04/noble-updates [amd64])
Inst openssl [3.0.13-0ubuntu3] (3.0.13-0ubuntu3.1 Ubuntu:24.04/noble-updates [amd64])
Conf curl (8.5.0-2ubuntu2 Ubuntu:24.04/noble-updates [amd64])
`)

	if got := countAptUpgrades(fixture); got != 3 {
		t.Errorf("countAptUpgrades = %d, want 3", got)
	}
	if got := countAptUpgrades([]byte("0 upgraded, 0 newly installed.\n")); got != 0 {
		t.Errorf("countAptUpgrades on an up-to-date machine = %d, want 0", got)
	}
}

func TestCountDnfUpdates(t *testing.T) {
	fixture := []byte(`Last metadata expiration check: 0:12:34 ago on Tue 02 Sep 2026.

kernel.x86_64                6.9.7-200.fc40                updates
openssl.x86_64               3.2.2-2.fc40                  updates

Obsoleting Packages
oldpkg.noarch                1.0-1.fc40                    updates
`)

	if got := countDnfUpdates(fixture); got != 2 {
		t.Errorf("countDnfUpdates = %d, want 2 (obsoleted packages are not updates)", got)
	}
}

func TestParseWindowsUpdates(t *testing.T) {
	withRegistry := []byte(`{"LastSuccess":"2026-08-14 03:11:52","LastHotFix":"/Date(1723600000000)/"}`)
	facts, err := parseWindowsUpdates(withRegistry)
	if err != nil {
		t.Fatalf("parseWindowsUpdates: %v", err)
	}
	if facts.LastInstalled.Format("2006-01-02") != "2026-08-14" {
		t.Errorf("last installed = %v, want the registry's record", facts.LastInstalled)
	}
	if facts.Pending != -1 {
		t.Errorf("pending = %d, want -1: the count is unknown without asking Windows Update", facts.Pending)
	}

	hotfixOnly := []byte(`{"LastSuccess":null,"LastHotFix":"/Date(1723600000000)/"}`)
	facts, err = parseWindowsUpdates(hotfixOnly)
	if err != nil {
		t.Fatalf("parseWindowsUpdates: %v", err)
	}
	if facts.LastInstalled.IsZero() || facts.Source != "installed hotfix list" {
		t.Errorf("facts = %+v, want the hotfix fallback", facts)
	}

	empty, err := parseWindowsUpdates([]byte(`{"LastSuccess":null,"LastHotFix":null}`))
	if err != nil {
		t.Fatalf("parseWindowsUpdates: %v", err)
	}
	if !empty.LastInstalled.IsZero() {
		t.Errorf("last installed = %v, want zero when nothing is recorded", empty.LastInstalled)
	}
}

func TestParseMacSoftwareUpdateDate(t *testing.T) {
	got := parseMacSoftwareUpdateDate([]byte("2026-08-01 10:22:31 +0000\n"))
	if got.IsZero() || got.Format("2006-01-02") != "2026-08-01" {
		t.Errorf("parseMacSoftwareUpdateDate = %v", got)
	}
	if got := parseMacSoftwareUpdateDate([]byte("")); !got.IsZero() {
		t.Errorf("empty input = %v, want zero", got)
	}
}

func TestVerdictThresholds(t *testing.T) {
	now := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	check := osUpdatesCheck{now: func() time.Time { return now }}

	tests := []struct {
		name  string
		facts updateFacts
		want  checks.Severity
	}{
		{"recent", updateFacts{LastInstalled: now.AddDate(0, 0, -5), Pending: 0}, checks.SeverityOK},
		{"recent with pending", updateFacts{LastInstalled: now.AddDate(0, 0, -5), Pending: 12}, checks.SeverityAttention},
		{"stale", updateFacts{LastInstalled: now.AddDate(0, 0, -90), Pending: -1}, checks.SeverityAttention},
		{"very stale", updateFacts{LastInstalled: now.AddDate(0, 0, -200), Pending: -1}, checks.SeverityUrgent},
		{"no record", updateFacts{Pending: -1}, checks.SeverityUnknown},
		{"no record but pending known", updateFacts{Pending: 4}, checks.SeverityAttention},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := check.verdict(tc.facts).Severity; got != tc.want {
				t.Errorf("severity = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestVerdictRecordsDaysSinceUpdate(t *testing.T) {
	now := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	check := osUpdatesCheck{now: func() time.Time { return now }}

	res := check.verdict(updateFacts{LastInstalled: now.AddDate(0, 0, -10), Pending: 0, Source: "apt package cache"})
	if got := res.Detail["days_since_update"]; got != 10 {
		t.Errorf("days_since_update = %v, want 10", got)
	}
	if got := res.Detail["source"]; got != "apt package cache" {
		t.Errorf("source = %v, want the collector's own words", got)
	}
}
