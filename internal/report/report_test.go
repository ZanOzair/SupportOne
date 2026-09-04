package report

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/explain"
	"github.com/ZanOzair/SupportOne/internal/i18n"
	"github.com/ZanOzair/SupportOne/internal/platform"
)

var update = flag.Bool("update", false, "rewrite the golden files from the current renderer")

// fixtureAdvice is the offline explanation for the fixture snapshot, so the
// golden file covers the advice block rather than only the verdicts.
func fixtureAdvice() map[string]explain.Advice {
	e := explain.New(nil, nil, platform.Windows)

	out := make(map[string]explain.Advice)
	for _, res := range fixtureSnapshot().Results {
		if advice, ok := e.For(res); ok {
			out[res.CheckID] = advice
		}
	}
	return out
}

func fixtureSnapshot() checks.Snapshot {
	generated := time.Date(2026, 9, 2, 13, 20, 0, 0, time.UTC)
	reallocated := 142

	return checks.Snapshot{
		Schema:       checks.SnapshotSchema,
		AgentVersion: "1.0.0",
		GeneratedAt:  generated,
		Host:         platform.Host{OS: platform.Windows, Arch: "amd64"},
		Results: []checks.Result{
			{
				CheckID:  "os.info",
				Severity: checks.SeverityOK,
				Summary:  "check.os.info.ok",
				Args:     []any{"Microsoft Windows 11 Pro", "10.0.22631", "2d 4h"},
				Detail:   map[string]any{"build": "22631", "kernel": "10.0.22631"},
			},
			{
				CheckID:  "disk.smart",
				Severity: checks.SeverityUrgent,
				Summary:  "check.disk.smart.bad_spots",
				Args:     []any{"Seagate ST2000", 142},
				Detail: map[string]any{
					"disks": []map[string]any{
						{"name": "Seagate ST2000", "status": "failing", "reallocated_sectors": reallocated},
					},
				},
			},
			{
				CheckID:  "updates.os",
				Severity: checks.SeverityAttention,
				Summary:  "check.updates.os.stale",
				Args:     []any{97},
				Detail:   map[string]any{"days_since_update": 97, "source": "Windows Update install history"},
			},
			{
				CheckID:  "eventlog.errors",
				Severity: checks.SeverityUnknown,
				Summary:  "check.unknown.failed",
				Err:      "the System log could not be read",
			},
		},
		SkippedAdmin: []string{"security.posture"},
	}
}

func TestHTMLMatchesGoldenFile(t *testing.T) {
	bundle, err := i18n.Load("en")
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}

	var got bytes.Buffer
	opts := Options{
		Bundle:    bundle,
		AuditPath: "/home/example/.config/SupportOne/audit.log",
		Advice:    fixtureAdvice(),
	}
	if err := HTML(&got, fixtureSnapshot(), opts); err != nil {
		t.Fatalf("HTML: %v", err)
	}

	golden := filepath.Join("testdata", "report.golden.html")
	if *update {
		if err := os.WriteFile(golden, got.Bytes(), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden (run: go test ./internal/report -update): %v", err)
	}
	if got.String() != string(want) {
		t.Errorf("rendered report differs from %s; rerun with -update if the change is intended", golden)
	}
}

func TestHTMLIsSelfContained(t *testing.T) {
	bundle, err := i18n.Load("en")
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}

	var out bytes.Buffer
	if err := HTML(&out, fixtureSnapshot(), Options{Bundle: bundle}); err != nil {
		t.Fatalf("HTML: %v", err)
	}

	rendered := out.String()
	// A report that fetches anything is a report that phones home when opened.
	for _, forbidden := range []string{"http://", "https://", "<script", "@import", "url("} {
		if strings.Contains(rendered, forbidden) {
			t.Errorf("report contains %q, so opening it would reach outside the file", forbidden)
		}
	}
}

func TestHTMLOrdersTheWorstFindingsFirst(t *testing.T) {
	bundle, err := i18n.Load("en")
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}

	var out bytes.Buffer
	if err := HTML(&out, fixtureSnapshot(), Options{Bundle: bundle}); err != nil {
		t.Fatalf("HTML: %v", err)
	}

	rendered := out.String()
	order := []string{"disk.smart", "updates.os", "eventlog.errors", "os.info"}
	previous := -1
	for _, id := range order {
		at := strings.Index(rendered, ">"+id+"<")
		if at < 0 {
			t.Fatalf("%s is missing from the report", id)
		}
		if at < previous {
			t.Errorf("%s appears before a more serious finding", id)
		}
		previous = at
	}
}

func TestHTMLEscapesCheckOutput(t *testing.T) {
	bundle, err := i18n.Load("en")
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}

	// Check output can contain anything the machine's own logs contain,
	// including something shaped like markup.
	snap := checks.Snapshot{
		Host: platform.Host{OS: platform.Linux, Arch: "amd64"},
		Results: []checks.Result{{
			CheckID:  "eventlog.errors",
			Severity: checks.SeverityOK,
			Summary:  "check.eventlog.errors.quiet",
			Args:     []any{2, 7},
			Detail:   map[string]any{"source": `<img src=x onerror="alert(1)">`},
		}},
	}

	var out bytes.Buffer
	if err := HTML(&out, snap, Options{Bundle: bundle}); err != nil {
		t.Fatalf("HTML: %v", err)
	}
	if strings.Contains(out.String(), "<img src=x") {
		t.Error("log content was rendered as markup rather than escaped")
	}
}

func TestJSONRoundTrips(t *testing.T) {
	var out bytes.Buffer
	if err := JSON(&out, fixtureSnapshot()); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if !strings.Contains(out.String(), `"check_id": "disk.smart"`) {
		t.Errorf("JSON report is missing a result:\n%s", out.String())
	}
}

func TestFilenameIsSortable(t *testing.T) {
	if got := Filename(fixtureSnapshot(), "html"); got != "supportone-2026-09-02-1320.html" {
		t.Errorf("Filename = %q", got)
	}
	if got := Filename(fixtureSnapshot(), ".json"); got != "supportone-2026-09-02-1320.json" {
		t.Errorf("Filename with a dotted extension = %q", got)
	}
}

func TestTheReportCarriesTheOfflineExplanation(t *testing.T) {
	bundle, err := i18n.Load("en")
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}

	var out bytes.Buffer
	if err := HTML(&out, fixtureSnapshot(), Options{Bundle: bundle, Advice: fixtureAdvice()}); err != nil {
		t.Fatalf("HTML: %v", err)
	}
	rendered := out.String()

	// A saved report is read away from the machine, so the advice has to
	// travel with it rather than living only in the interface.
	for _, want := range []string{"What this means:", "What to do:", "Copy anything you would hate to lose"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("the report is missing %q", want)
		}
	}
}

func TestAReportWithoutAdviceStillRendersEveryVerdict(t *testing.T) {
	bundle, err := i18n.Load("en")
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}

	var out bytes.Buffer
	if err := HTML(&out, fixtureSnapshot(), Options{Bundle: bundle}); err != nil {
		t.Fatalf("HTML: %v", err)
	}
	rendered := out.String()

	if strings.Contains(rendered, "What this means:") {
		t.Error("the advice block was rendered with no advice to put in it")
	}
	for _, id := range []string{"os.info", "disk.smart", "updates.os"} {
		if !strings.Contains(rendered, ">"+id+"<") {
			t.Errorf("%s is missing from a report rendered without advice", id)
		}
	}
}

func TestTheAdviceIsEscapedLikeEverythingElse(t *testing.T) {
	bundle, err := i18n.Load("en")
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}

	snap := checks.Snapshot{
		Schema:  checks.SnapshotSchema,
		Results: []checks.Result{{CheckID: "os.info", Severity: checks.SeverityOK, Summary: "check.os.info.ok"}},
	}
	advice := map[string]explain.Advice{
		"os.info": {CheckID: "os.info", Cause: "<img src=x onerror=alert(1)>", Steps: []string{"<script>bad()</script>"}},
	}

	var out bytes.Buffer
	if err := HTML(&out, snap, Options{Bundle: bundle, Advice: advice}); err != nil {
		t.Fatalf("HTML: %v", err)
	}
	rendered := out.String()

	if strings.Contains(rendered, "<img src=x") || strings.Contains(rendered, "<script>bad()") {
		t.Error("advice was rendered as markup rather than escaped")
	}
}
