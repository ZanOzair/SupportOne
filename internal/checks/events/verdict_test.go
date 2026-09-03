package events

import (
	"testing"
	"time"

	"github.com/ZanOzair/SupportOne/internal/checks"
)

const week = 7 * 24 * time.Hour

func repeated(source string, times int) []logEvent {
	out := make([]logEvent, times)
	for i := range out {
		out[i] = logEvent{Source: source, ID: "153", Level: levelError}
	}
	return out
}

func TestEventsVerdict(t *testing.T) {
	scattered := []logEvent{
		{Source: "bluetooth", ID: "17", Level: levelError},
		{Source: "cups", ID: "4", Level: levelError},
		{Source: "kernel", ID: "9", Level: levelError},
	}

	tests := []struct {
		name   string
		events []logEvent
		want   checks.Severity
	}{
		{"a quiet week", nil, checks.SeverityOK},
		{"errors that do not repeat", scattered, checks.SeverityOK},
		{"nine repeats is still noise", repeated("disk", 9), checks.SeverityOK},
		{"ten repeats is a pattern", repeated("disk", 10), checks.SeverityAttention},
		{"a critical event", []logEvent{{Source: "Kernel-Power", ID: "41", Level: levelCritical}}, checks.SeverityAttention},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := eventsVerdict(tc.events, week).Severity; got != tc.want {
				t.Errorf("severity = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEventsVerdictDoesNotPresentACountAsAProblemList(t *testing.T) {
	// Forty scattered errors is what a working machine looks like. Reporting
	// that as forty problems is the pattern this check exists to avoid.
	var many []logEvent
	for i := 0; i < 40; i++ {
		many = append(many, logEvent{Source: "source" + string(rune('a'+i%20)), ID: "1", Level: levelError})
	}

	res := eventsVerdict(many, week)
	if res.Severity != checks.SeverityOK {
		t.Errorf("severity = %q, want ok for scattered errors", res.Severity)
	}
	if res.Detail["count"] != 40 {
		t.Errorf("count = %v, want the number still reported as evidence", res.Detail["count"])
	}
}

func TestEventsVerdictReportsTheWindowItActuallyLookedAt(t *testing.T) {
	// macOS uses a shorter window, so a quiet day must not read as a quiet week.
	res := eventsVerdict(nil, 24*time.Hour)
	if res.Args[0] != 1 {
		t.Errorf("window = %v days, want 1", res.Args[0])
	}
}

func TestEventsVerdictNamesTheWorstRepeat(t *testing.T) {
	events := append(repeated("disk", 12), repeated("bluetooth", 10)...)

	res := eventsVerdict(events, week)
	if res.Args[0] != "disk" || res.Args[1] != 12 {
		t.Errorf("args = %v, want the most frequent source first", res.Args)
	}
}
