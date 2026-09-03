package redact

import (
	"strings"
	"testing"

	"github.com/ZanOzair/SupportOne/internal/checks"
)

func testIdentity() Identity {
	return Identity{Hostname: "alex-laptop", Username: "alex", HomeDir: "/home/alex"}
}

func testSnapshot() checks.Snapshot {
	return checks.Snapshot{
		Results: []checks.Result{
			{
				CheckID:  "network.config",
				Severity: checks.SeverityOK,
				Summary:  "check.network.config.ok",
				Args:     []any{"eth0", "192.168.1.1"},
				Detail: map[string]any{
					"gateway": "192.168.1.1",
					"dns":     []string{"192.168.1.1", "1.1.1.1"},
					"interfaces": []map[string]any{
						{"name": "eth0", "mac": "a4:83:e7:11:22:33", "addresses": []string{"192.168.1.20/24"}},
					},
				},
			},
			{
				CheckID:  "disk.smart",
				Severity: checks.SeverityUnknown,
				Summary:  "check.unknown.failed",
				Err:      "open /home/alex/.config/SupportOne/audit.log: permission denied",
				Detail: map[string]any{
					"disks": []map[string]any{
						{"name": "disk0", "serial_number": "S5H7NS0R123456", "status": "healthy"},
					},
					"hostname": "alex-laptop",
				},
			},
		},
	}
}

func TestEverythingRedactsIdentifyingDetail(t *testing.T) {
	snap, err := Everything().Snapshot(testSnapshot(), testIdentity())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	rendered := render(t, snap)
	for _, secret := range []string{
		"192.168.1.1", "192.168.1.20", "a4:83:e7:11:22:33",
		"S5H7NS0R123456", "alex-laptop", "/home/alex", "alex",
	} {
		if strings.Contains(rendered, secret) {
			t.Errorf("redacted snapshot still contains %q:\n%s", secret, rendered)
		}
	}
	if !strings.Contains(rendered, Marker) {
		t.Error("nothing was marked as redacted; the reader cannot tell something was removed")
	}
}

func TestRedactionKeepsTheDiagnosisIntact(t *testing.T) {
	snap, err := Everything().Snapshot(testSnapshot(), testIdentity())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	// Redaction removes who and where, never what the check found. A report
	// stripped of its verdicts would be useless to the technician receiving it.
	rendered := render(t, snap)
	for _, kept := range []string{"network.config", "check.network.config.ok", "disk.smart", "healthy", "eth0"} {
		if !strings.Contains(rendered, kept) {
			t.Errorf("redaction removed %q, which is diagnosis rather than identity:\n%s", kept, rendered)
		}
	}
	if snap.Results[0].Severity != checks.SeverityOK {
		t.Errorf("severity changed to %q", snap.Results[0].Severity)
	}
}

func TestPolicySelectsWhatIsRemoved(t *testing.T) {
	onlyAddresses := Policy{Addresses: true}
	snap, err := onlyAddresses.Snapshot(testSnapshot(), testIdentity())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	rendered := render(t, snap)
	if strings.Contains(rendered, "192.168.1.20") {
		t.Error("an address survived an addresses-only policy")
	}
	if !strings.Contains(rendered, "S5H7NS0R123456") {
		t.Error("a serial was removed by an addresses-only policy")
	}
	if !strings.Contains(rendered, "alex-laptop") {
		t.Error("a hostname was removed by an addresses-only policy")
	}
}

func TestNothingPolicyLeavesTheSnapshotAlone(t *testing.T) {
	original := testSnapshot()
	snap, err := Policy{}.Snapshot(original, testIdentity())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if render(t, snap) != render(t, original) {
		t.Error("the zero policy changed the snapshot; redaction must always be an explicit choice")
	}
}

func TestSnapshotDoesNotModifyTheOriginal(t *testing.T) {
	original := testSnapshot()
	before := render(t, original)

	if _, err := Everything().Snapshot(original, testIdentity()); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if after := render(t, original); after != before {
		t.Error("redaction modified the snapshot in place; the on-screen report would change under the user")
	}
}

func TestReplaceFoldIgnoresCaseAndVeryShortNames(t *testing.T) {
	got := replaceFold(`C:\Users\Alex\Desktop and /home/alex/docs`, "alex", Marker)
	if strings.Contains(strings.ToLower(got), "alex") {
		t.Errorf("case-different occurrence survived: %q", got)
	}

	// A two-character username would mangle unrelated words.
	unchanged := replaceFold("an ordinary sentence", "an", Marker)
	if unchanged != "an ordinary sentence" {
		t.Errorf("a two-character name was replaced: %q", unchanged)
	}
}

func TestUsernameInsidePathIsRedactedAsAWhole(t *testing.T) {
	snap, err := Everything().Snapshot(testSnapshot(), testIdentity())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if got := snap.Results[1].Err; strings.Contains(got, "alex") {
		t.Errorf("error message still names the user: %q", got)
	}
}

func render(t *testing.T, snap checks.Snapshot) string {
	t.Helper()
	var b strings.Builder
	if err := jsonEncode(&b, snap); err != nil {
		t.Fatalf("encode snapshot: %v", err)
	}
	return b.String()
}
