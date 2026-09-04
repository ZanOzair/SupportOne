package patches

import (
	"testing"
	"time"

	"github.com/ZanOzair/SupportOne/internal/checks"
)

var now = time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

func ago(d time.Duration) time.Time { return now.Add(-d) }

func TestPatchesVerdict(t *testing.T) {
	cases := []struct {
		name     string
		facts    patchFacts
		severity checks.Severity
		summary  string
	}{
		{
			"two this month",
			patchFacts{Source: "dpkg log", Applied: []patch{
				{ID: "curl", Applied: ago(3 * 24 * time.Hour)},
				{ID: "libc6", Applied: ago(10 * 24 * time.Hour)},
			}},
			checks.SeverityOK, keyPatchesRecent,
		},
		{
			"exactly one this month, so the singular",
			patchFacts{Source: "dpkg log", Applied: []patch{
				{ID: "curl", Applied: ago(3 * 24 * time.Hour)},
				{ID: "libc6", Applied: ago(200 * 24 * time.Hour)},
			}},
			checks.SeverityOK, keyPatchesRecentOne,
		},
		{
			"nothing in the last month",
			patchFacts{Source: "dpkg log", Applied: []patch{
				{ID: "curl", Applied: ago(90 * 24 * time.Hour)},
			}},
			checks.SeverityAttention, keyPatchesOld,
		},
		{
			"a record that exists and is empty",
			patchFacts{Source: "dpkg log"},
			checks.SeverityAttention, keyPatchesNone,
		},
		{
			"no record at all",
			patchFacts{},
			checks.SeverityUnknown, keyPatchesUnreadable,
		},
		{
			// Entries with no dates cannot answer "how recently", so the
			// check says it could not read a dated record rather than
			// implying the machine is behind.
			"entries with no dates",
			patchFacts{Source: "rpm database", Applied: []patch{{ID: "curl"}, {ID: "bash"}}},
			checks.SeverityUnknown, keyPatchesUnreadable,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := patchesVerdict(tc.facts, now)
			if got.Severity != tc.severity {
				t.Errorf("severity = %q, want %q", got.Severity, tc.severity)
			}
			if got.Summary != tc.summary {
				t.Errorf("summary = %q, want %q", got.Summary, tc.summary)
			}
		})
	}
}

// TestAPatchRecordIsNeverUrgent keeps this check from second-guessing
// updates.os. Whether a machine is dangerously behind is that check's
// question; answering it twice would let the two disagree in one report.
func TestAPatchRecordIsNeverUrgent(t *testing.T) {
	ancient := patchFacts{Source: "dpkg log", Applied: []patch{
		{ID: "curl", Applied: ago(5 * 365 * 24 * time.Hour)},
	}}

	if got := patchesVerdict(ancient, now); got.Severity == checks.SeverityUrgent {
		t.Errorf("a five-year-old patch record was reported as urgent: %+v", got)
	}
}

func TestTheEvidenceCarriesTheDatedList(t *testing.T) {
	facts := patchFacts{
		Source:  "Windows servicing record",
		Horizon: ago(400 * 24 * time.Hour),
		Applied: []patch{
			{ID: "KB5041585", Title: "Security Update", Applied: ago(3 * 24 * time.Hour)},
			{ID: "KB5039895", Applied: ago(60 * 24 * time.Hour)},
		},
	}

	got := patchesVerdict(facts, now)
	if got.Detail["source"] != "Windows servicing record" {
		t.Errorf("the evidence does not name the record it read: %v", got.Detail)
	}
	if got.Detail["recorded"] != 2 {
		t.Errorf("recorded = %v, want 2", got.Detail["recorded"])
	}
	if _, ok := got.Detail["record_starts"]; !ok {
		t.Error("the evidence does not say how far back the record reaches")
	}

	listed, ok := got.Detail["patches"].([]map[string]any)
	if !ok || len(listed) != 2 {
		t.Fatalf("patches = %v, want two entries", got.Detail["patches"])
	}
	// Newest first: a reader scanning the list wants the recent ones on top.
	if listed[0]["id"] != "KB5041585" {
		t.Errorf("first entry = %v, want the newest", listed[0])
	}
	if _, hasTitle := listed[1]["title"]; hasTitle {
		t.Error("an entry with no description carries an empty one")
	}
}

func TestTheListIsCappedButTheCountIsNot(t *testing.T) {
	var applied []patch
	for i := 0; i < maxListed+25; i++ {
		applied = append(applied, patch{ID: "pkg", Applied: ago(time.Duration(i) * time.Hour)})
	}

	got := patchesVerdict(patchFacts{Source: "dpkg log", Applied: applied}, now)

	if got.Detail["recorded"] != maxListed+25 {
		t.Errorf("recorded = %v, want the full count", got.Detail["recorded"])
	}
	listed := got.Detail["patches"].([]map[string]any)
	if len(listed) != maxListed {
		t.Errorf("listed %d entries, want the cap of %d", len(listed), maxListed)
	}
}

func TestUndatedEntriesSortLast(t *testing.T) {
	got := newestFirst([]patch{
		{ID: "undated"},
		{ID: "old", Applied: ago(90 * 24 * time.Hour)},
		{ID: "new", Applied: ago(1 * time.Hour)},
	})

	want := []string{"new", "old", "undated"}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("position %d = %q, want %q", i, got[i].ID, id)
		}
	}
}

func TestCountSinceIgnoresUndatedEntries(t *testing.T) {
	// An unknown date is not evidence of recent activity.
	applied := []patch{{ID: "undated"}, {ID: "recent", Applied: ago(time.Hour)}}

	if got := countSince(applied, now.Add(-recentWindow)); got != 1 {
		t.Errorf("countSince = %d, want 1", got)
	}
}

func TestOldest(t *testing.T) {
	earliest := ago(400 * 24 * time.Hour)
	applied := []patch{
		{ID: "a", Applied: ago(3 * time.Hour)},
		{ID: "b", Applied: earliest},
		{ID: "undated"},
	}

	if got := oldest(applied); !got.Equal(earliest) {
		t.Errorf("oldest = %v, want %v", got, earliest)
	}
	if got := oldest([]patch{{ID: "undated"}}); !got.IsZero() {
		t.Errorf("oldest of undated entries = %v, want the zero time", got)
	}
}

func TestTheCheckDeclaresItself(t *testing.T) {
	c := installedCheck{}
	if c.ID() != "updates.installed" {
		t.Errorf("ID = %q", c.ID())
	}
	if c.RequiresAdmin() {
		t.Error("RequiresAdmin = true; every record this reads is world-readable")
	}
	if len(c.Platforms()) != 3 {
		t.Errorf("Platforms = %v, want all three", c.Platforms())
	}
}
