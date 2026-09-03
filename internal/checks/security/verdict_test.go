package security

import (
	"testing"

	"github.com/ZanOzair/SupportOne/internal/checks"
)

func TestPostureVerdict(t *testing.T) {
	tests := []struct {
		name  string
		facts postureFacts
		want  checks.Severity
	}{
		{
			name:  "everything on",
			facts: postureFacts{DiskEncryption: stateOn, Firewall: stateOn, Antivirus: stateOn},
			want:  checks.SeverityOK,
		},
		{
			name:  "encryption off",
			facts: postureFacts{DiskEncryption: stateOff, Firewall: stateOn, Antivirus: stateOn},
			want:  checks.SeverityAttention,
		},
		{
			name:  "two protections off",
			facts: postureFacts{DiskEncryption: stateOff, Firewall: stateOff, Antivirus: stateOn},
			want:  checks.SeverityAttention,
		},
		{
			name:  "nothing readable",
			facts: postureFacts{DiskEncryption: stateUnknown, Firewall: stateUnknown, Antivirus: stateUnknown},
			want:  checks.SeverityUnknown,
		},
		{
			name: "unknown encryption is not a finding on its own",
			// BitLocker without administrator rights reads as unknown. That
			// must not be reported as an unencrypted disk.
			facts: postureFacts{DiskEncryption: stateUnknown, Firewall: stateOn, Antivirus: stateOn},
			want:  checks.SeverityOK,
		},
		{
			name: "antivirus that does not apply is not antivirus that is off",
			facts: postureFacts{
				DiskEncryption: stateOn, Firewall: stateOn, Antivirus: stateNotApplicable,
			},
			want: checks.SeverityOK,
		},
		{
			name: "a readable firewall keeps an unknown-heavy posture from reading as unknown",
			facts: postureFacts{
				DiskEncryption: stateUnknown, Firewall: stateOn, Antivirus: stateNotApplicable,
			},
			want: checks.SeverityOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := postureVerdict(tc.facts).Severity; got != tc.want {
				t.Errorf("severity = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPostureVerdictCountsWhatIsOff(t *testing.T) {
	res := postureVerdict(postureFacts{
		DiskEncryption: stateOff, Firewall: stateOff, Antivirus: stateOff,
	})
	if res.Args[0] != 3 {
		t.Errorf("count = %v, want 3", res.Args[0])
	}
}

func TestPostureVerdictCarriesTheReasonsItCouldNotRead(t *testing.T) {
	res := postureVerdict(postureFacts{
		DiskEncryption: stateOn,
		Firewall:       stateUnknown,
		Antivirus:      stateNotApplicable,
		Notes:          map[string]string{"firewall": "reading the firewall rules on Linux requires administrator rights"},
	})

	notes, ok := res.Detail["notes"].(map[string]string)
	if !ok || notes["firewall"] == "" {
		t.Errorf("detail = %v, want the collector's reason preserved", res.Detail)
	}
}
