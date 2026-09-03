package drivers

import (
	"context"
	"testing"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/platform"
)

func TestProblemVerdict(t *testing.T) {
	if got := problemVerdict(nil); got.Severity != checks.SeverityOK || got.Summary != keyDriversOK {
		t.Errorf("no problem devices = %+v, want a plain OK", got)
	}

	res := problemVerdict([]device{
		{Name: "Realtek Audio", ErrorCode: 10, Meaning: "cannot start"},
		{Name: "Fingerprint Reader", ErrorCode: 28},
	})
	if res.Severity != checks.SeverityAttention {
		t.Errorf("severity = %q, want attention", res.Severity)
	}
	if res.Args[0] != 2 || res.Args[1] != "Realtek Audio, Fingerprint Reader" {
		t.Errorf("args = %v, want the count and the device names", res.Args)
	}
}

func TestProblemVerdictUsesTheSingularForOneDevice(t *testing.T) {
	res := problemVerdict([]device{{Name: "Realtek Audio", ErrorCode: 10}})
	if res.Summary != keyDriversProblem+".one" {
		t.Errorf("summary = %q, want the singular variant", res.Summary)
	}
}

func TestCheckIsOfferedOnWindowsOnly(t *testing.T) {
	platforms := problemCheck{}.Platforms()
	if len(platforms) != 1 || platforms[0] != platform.Windows {
		t.Errorf("platforms = %v, want Windows only", platforms)
	}
}

func TestRunOffWindowsSaysSoRatherThanReportingNoProblems(t *testing.T) {
	if platform.Current() == platform.Windows {
		t.Skip("this covers the behaviour on the platforms the check does not serve")
	}

	// The registry never offers this check off Windows, but if it were called
	// it must not claim every device is fine on a platform it never asked.
	res, err := problemCheck{}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Severity != checks.SeverityUnknown {
		t.Errorf("severity = %q, want unknown", res.Severity)
	}
}
