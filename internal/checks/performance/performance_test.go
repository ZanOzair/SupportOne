package performance

import (
	"strings"
	"testing"

	"github.com/ZanOzair/SupportOne/internal/checks"
)

const gib = 1 << 30

func TestLoadVerdictMemoryPressure(t *testing.T) {
	cases := []struct {
		name     string
		facts    loadFacts
		severity checks.Severity
		summary  string
	}{
		{
			"plenty free",
			loadFacts{Cores: 8, MemTotalBytes: 16 * gib, MemAvailableBytes: 10 * gib},
			checks.SeverityOK, keyLoadOK,
		},
		{
			"just above the low threshold",
			// 20.1% available: the threshold is a stated number, not a mood.
			loadFacts{Cores: 8, MemTotalBytes: 1000, MemAvailableBytes: 201},
			checks.SeverityOK, keyLoadOK,
		},
		{
			"just below the low threshold",
			loadFacts{Cores: 8, MemTotalBytes: 1000, MemAvailableBytes: 199},
			checks.SeverityAttention, keyLoadMemoryLow,
		},
		{
			"just below the critical threshold",
			loadFacts{Cores: 8, MemTotalBytes: 1000, MemAvailableBytes: 99},
			checks.SeverityUrgent, keyLoadMemoryCritical,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := loadVerdict(tc.facts)
			if got.Severity != tc.severity {
				t.Errorf("severity = %q, want %q", got.Severity, tc.severity)
			}
			if got.Summary != tc.summary {
				t.Errorf("summary = %q, want %q", got.Summary, tc.summary)
			}
		})
	}
}

// TestABusyProcessorIsNeverUrgent is the honesty rule this check is built
// around: one reading of a busy machine is not evidence of a fault.
func TestABusyProcessorIsNeverUrgent(t *testing.T) {
	extremes := []loadFacts{
		{Cores: 1, LoadAverage: 64, HasLoadAverage: true, MemTotalBytes: 16 * gib, MemAvailableBytes: 15 * gib},
		{Cores: 8, BusyPercent: 100, HasBusy: true, MemTotalBytes: 16 * gib, MemAvailableBytes: 15 * gib},
	}

	for _, facts := range extremes {
		got := loadVerdict(facts)
		if got.Severity == checks.SeverityUrgent {
			t.Errorf("a busy processor was reported as urgent: %+v", got)
		}
		if got.Severity != checks.SeverityAttention {
			t.Errorf("severity = %q, want attention", got.Severity)
		}
	}
}

func TestLoadVerdictProcessorPressure(t *testing.T) {
	roomy := loadFacts{MemTotalBytes: 16 * gib, MemAvailableBytes: 15 * gib}

	cases := []struct {
		name     string
		facts    loadFacts
		severity checks.Severity
		summary  string
	}{
		{
			"a quiet run queue",
			loadFacts{Cores: 8, LoadAverage: 1.2, HasLoadAverage: true},
			checks.SeverityOK, keyLoadOK,
		},
		{
			"at the busy threshold",
			loadFacts{Cores: 4, LoadAverage: 8.0, HasLoadAverage: true},
			checks.SeverityAttention, keyLoadBusy,
		},
		{
			"just under it",
			loadFacts{Cores: 4, LoadAverage: 7.9, HasLoadAverage: true},
			checks.SeverityOK, keyLoadOK,
		},
		{
			"an instantaneous Windows reading",
			loadFacts{Cores: 8, BusyPercent: 95, HasBusy: true},
			checks.SeverityAttention, keyLoadBusyNow,
		},
		{
			"an idle Windows reading",
			loadFacts{Cores: 8, BusyPercent: 3, HasBusy: true},
			checks.SeverityOK, keyLoadOK,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			facts := tc.facts
			facts.MemTotalBytes, facts.MemAvailableBytes = roomy.MemTotalBytes, roomy.MemAvailableBytes

			got := loadVerdict(facts)
			if got.Severity != tc.severity {
				t.Errorf("severity = %q, want %q", got.Severity, tc.severity)
			}
			if got.Summary != tc.summary {
				t.Errorf("summary = %q, want %q", got.Summary, tc.summary)
			}
		})
	}
}

func TestSwapPressureIsReportedWhenMemoryStillLooksFine(t *testing.T) {
	facts := loadFacts{
		Cores:             8,
		MemTotalBytes:     16 * gib,
		MemAvailableBytes: 8 * gib,
		SwapTotalBytes:    4 * gib,
		SwapUsedBytes:     3 * gib,
	}

	got := loadVerdict(facts)
	if got.Severity != checks.SeverityAttention || got.Summary != keyLoadSwapping {
		t.Errorf("got %q/%q, want attention/%s", got.Severity, got.Summary, keyLoadSwapping)
	}
}

func TestNoSwapConfiguredIsNotPressure(t *testing.T) {
	facts := loadFacts{Cores: 8, MemTotalBytes: 16 * gib, MemAvailableBytes: 8 * gib}
	if swapping(facts) {
		t.Error("a machine with no swap was reported as swapping")
	}
	if got := loadVerdict(facts); got.Severity != checks.SeverityOK {
		t.Errorf("severity = %q, want ok", got.Severity)
	}
}

func TestAMachineThatReportedNothingIsUnknownNotOK(t *testing.T) {
	got := loadVerdict(loadFacts{Cores: 4})
	if got.Severity != checks.SeverityUnknown {
		t.Errorf("severity = %q, want unknown", got.Severity)
	}
	if got.Summary != keyLoadUnreadable {
		t.Errorf("summary = %q, want %q", got.Summary, keyLoadUnreadable)
	}
}

func TestTheVerdictCarriesItsEvidence(t *testing.T) {
	facts := loadFacts{
		Cores: 8, LoadAverage: 1.5, HasLoadAverage: true,
		MemTotalBytes: 16 * gib, MemAvailableBytes: 8 * gib,
		SwapTotalBytes: 2 * gib, SwapUsedBytes: 1 << 20,
	}

	got := loadVerdict(facts)
	for _, key := range []string{"cores", "load_average_1m", "memory_total_bytes", "memory_available_bytes", "swap_total_bytes"} {
		if _, ok := got.Detail[key]; !ok {
			t.Errorf("the evidence does not carry %q: %v", key, got.Detail)
		}
	}
	// An instantaneous reading was not taken on this platform, so it must not
	// appear as though it were zero.
	if _, ok := got.Detail["busy_percent"]; ok {
		t.Error("the evidence claims a busy percentage this platform did not report")
	}
}

func TestTheCheckDeclaresItself(t *testing.T) {
	c := loadCheck{}
	if c.ID() != "performance.load" {
		t.Errorf("ID = %q", c.ID())
	}
	if c.RequiresAdmin() {
		t.Error("RequiresAdmin = true for a check that reads public counters")
	}
	if len(c.Platforms()) != 3 {
		t.Errorf("Platforms = %v, want all three", c.Platforms())
	}
	if !strings.HasPrefix(keyLoadOK, "check.performance.") {
		t.Errorf("message keys are not namespaced to the check: %q", keyLoadOK)
	}
}
