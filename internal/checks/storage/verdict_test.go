package storage

import (
	"testing"

	"github.com/ZanOzair/SupportOne/internal/checks"
)

func TestVolumesVerdictThresholds(t *testing.T) {
	tests := []struct {
		name    string
		volumes []volume
		want    checks.Severity
	}{
		{"plenty of room", []volume{{Mount: "/", TotalBytes: 100, FreeBytes: 60}}, checks.SeverityOK},
		{"exactly at the low threshold", []volume{{Mount: "/", TotalBytes: 100, FreeBytes: 10}}, checks.SeverityOK},
		{"below the low threshold", []volume{{Mount: "/", TotalBytes: 100, FreeBytes: 9}}, checks.SeverityAttention},
		{"exactly at the critical threshold", []volume{{Mount: "/", TotalBytes: 100, FreeBytes: 5}}, checks.SeverityAttention},
		{"below the critical threshold", []volume{{Mount: "/", TotalBytes: 100, FreeBytes: 4}}, checks.SeverityUrgent},
		{"nothing readable", nil, checks.SeverityUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := volumesVerdict(tc.volumes).Severity; got != tc.want {
				t.Errorf("severity = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestVolumesVerdictJudgesTheTightestVolume(t *testing.T) {
	res := volumesVerdict([]volume{
		{Mount: "/", TotalBytes: 1000, FreeBytes: 800},
		{Mount: "/data", TotalBytes: 1000, FreeBytes: 20},
	})

	if res.Severity != checks.SeverityUrgent {
		t.Fatalf("severity = %q, want urgent — a roomy system drive must not mask a full one", res.Severity)
	}
	if res.Args[0] != "/data" {
		t.Errorf("the message names %v, want /data", res.Args[0])
	}
}

func TestDisksVerdict(t *testing.T) {
	none := 0
	bad := 142

	tests := []struct {
		name  string
		disks []disk
		want  checks.Severity
	}{
		{"all healthy", []disk{{Name: "disk0", Status: statusHealthy, Reallocated: &none}}, checks.SeverityOK},
		{"one failing", []disk{
			{Name: "disk0", Status: statusHealthy},
			{Name: "disk1", Status: statusFailing},
		}, checks.SeverityUrgent},
		{"retired sectors but no failure verdict", []disk{
			{Name: "disk0", Status: statusHealthy, Reallocated: &bad},
		}, checks.SeverityAttention},
		{"nothing reports a verdict", []disk{{Name: "disk0", Status: statusUnknown}}, checks.SeverityUnknown},
		{"no drives found", nil, checks.SeverityUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := disksVerdict(tc.disks).Severity; got != tc.want {
				t.Errorf("severity = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDisksVerdictNamesTheDriveWithBadSectors(t *testing.T) {
	bad := 142
	res := disksVerdict([]disk{
		{Name: "disk0", Status: statusHealthy},
		{Name: "disk1", Status: statusHealthy, Reallocated: &bad},
	})

	if res.Args[0] != "disk1" || res.Args[1] != 142 {
		t.Errorf("message args = %v, want the failing drive and its count", res.Args)
	}
}

func TestAMixOfHealthyAndUnknownIsNotUnknown(t *testing.T) {
	// One drive that cannot report should not erase a healthy verdict from
	// the others, nor should it read as a fault.
	res := disksVerdict([]disk{
		{Name: "disk0", Status: statusHealthy},
		{Name: "usb0", Status: statusUnknown},
	})
	if res.Severity != checks.SeverityOK {
		t.Errorf("severity = %q, want ok", res.Severity)
	}
}
