package schedule

import (
	"encoding/json"
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

func snapshot() checks.Snapshot {
	return checks.Snapshot{
		Schema:       checks.SnapshotSchema,
		AgentVersion: "1.0.0",
		GeneratedAt:  time.Date(2026, 8, 31, 23, 45, 0, 0, time.UTC),
		Host:         platform.Host{OS: platform.Linux, Arch: "amd64"},
		Results: []checks.Result{
			{CheckID: "os.info", Severity: checks.SeverityOK, Summary: "check.os.info.ok", Args: []any{"Debian", "12", "3d 2h"}},
			{CheckID: "disk.smart", Severity: checks.SeverityUrgent, Summary: "check.disk.smart.failing", Args: []any{"WD Blue"}},
		},
	}
}

func bundle(t *testing.T) *i18n.Bundle {
	t.Helper()

	b, err := i18n.Load("en")
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}
	return b
}

func advice() map[string]explain.Advice {
	e := explain.New(nil, nil, platform.Linux)

	out := make(map[string]explain.Advice)
	for _, a := range e.ForSnapshot(snapshot()) {
		out[a.CheckID] = a
	}
	return out
}

func TestWriteProducesBothReportsForTheMonth(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "reports")

	got, err := Write(snapshot(), Options{Dir: dir, Bundle: bundle(t), Advice: advice(), Redacted: true})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	// The period comes from the snapshot, not the clock: a report generated
	// late still covers the month it describes.
	if got.Period != "2026-08" {
		t.Errorf("Period = %q, want 2026-08", got.Period)
	}
	if filepath.Base(got.HTML) != "supportone-2026-08.html" {
		t.Errorf("HTML = %q", got.HTML)
	}
	if filepath.Base(got.JSON) != "supportone-2026-08.json" {
		t.Errorf("JSON = %q", got.JSON)
	}

	html, err := os.ReadFile(got.HTML)
	if err != nil {
		t.Fatalf("read the HTML report: %v", err)
	}
	rendered := string(html)

	// A report read weeks later still has to say what its findings mean.
	if !strings.Contains(rendered, "What this means:") {
		t.Error("the monthly report carries no explanation")
	}
	if !strings.Contains(rendered, "Copy anything you would hate to lose") {
		t.Error("the monthly report carries no next steps")
	}
	// And it opens with no network, which is what makes it postable.
	for _, forbidden := range []string{"http://", "https://", "<script"} {
		if strings.Contains(rendered, forbidden) {
			t.Errorf("the report contains %q, so opening it would reach outside the file", forbidden)
		}
	}

	raw, err := os.ReadFile(got.JSON)
	if err != nil {
		t.Fatalf("read the JSON report: %v", err)
	}
	var decoded checks.Snapshot
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("the JSON report is not a snapshot: %v", err)
	}
	// Both forms come from one snapshot, so they cannot disagree.
	if len(decoded.Results) != len(snapshot().Results) {
		t.Errorf("the JSON report holds %d results, want %d", len(decoded.Results), len(snapshot().Results))
	}
}

func TestARerunReplacesThatMonthRatherThanPilingUp(t *testing.T) {
	dir := t.TempDir()

	for i := 0; i < 3; i++ {
		if _, err := Write(snapshot(), Options{Dir: dir, Bundle: bundle(t)}); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("the folder holds %d files, want the month's two", len(entries))
	}
	// No temporary file survives a run for a reader to trip over.
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("a temporary file was left behind: %s", e.Name())
		}
	}
}

func TestEachMonthGetsItsOwnPairOfFiles(t *testing.T) {
	dir := t.TempDir()

	for _, month := range []time.Month{time.June, time.July, time.August} {
		snap := snapshot()
		snap.GeneratedAt = time.Date(2026, month, 15, 10, 0, 0, 0, time.UTC)
		if _, err := Write(snap, Options{Dir: dir, Bundle: bundle(t)}); err != nil {
			t.Fatalf("Write %v: %v", month, err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 6 {
		t.Errorf("the folder holds %d files, want two for each of three months", len(entries))
	}
}

func TestWriteNeedsSomewhereToWriteAndALanguage(t *testing.T) {
	if _, err := Write(snapshot(), Options{Dir: t.TempDir()}); err == nil {
		t.Error("Write accepted no language bundle")
	}
	if _, err := Write(snapshot(), Options{Bundle: bundle(t)}); err == nil {
		t.Error("Write accepted no folder")
	}
	if _, err := Write(snapshot(), Options{Dir: "   ", Bundle: bundle(t)}); err == nil {
		t.Error("Write accepted a whitespace folder")
	}
}

func TestASnapshotWithNoDateFallsBackToTheClock(t *testing.T) {
	dir := t.TempDir()

	snap := snapshot()
	snap.GeneratedAt = time.Time{}

	got, err := Write(snap, Options{
		Dir:    dir,
		Bundle: bundle(t),
		Now:    func() time.Time { return time.Date(2026, 12, 1, 7, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got.Period != "2026-12" {
		t.Errorf("Period = %q, want the clock's month", got.Period)
	}
}

// TestAFailedRenderLeavesLastMonthIntact is what the write-then-rename is for:
// a run that fails part-way must not destroy a report that was already there.
func TestAFailedRenderLeavesLastMonthIntact(t *testing.T) {
	dir := t.TempDir()

	if _, err := Write(snapshot(), Options{Dir: dir, Bundle: bundle(t)}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	good, err := os.ReadFile(filepath.Join(dir, "supportone-2026-08.html"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// A render that fails: writeFile must clean up and leave the previous
	// report exactly as it was.
	err = writeFile(filepath.Join(dir, "supportone-2026-08.html"), func(*os.File) error {
		return errRender
	})
	if err == nil {
		t.Fatal("writeFile reported success though the render failed")
	}

	after, err := os.ReadFile(filepath.Join(dir, "supportone-2026-08.html"))
	if err != nil {
		t.Fatalf("the previous report is gone: %v", err)
	}
	if string(after) != string(good) {
		t.Error("the previous report was changed by a failed run")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("a failed run left a temporary file behind: %s", e.Name())
		}
	}
}

func TestAFolderThatCannotBeWrittenIsReported(t *testing.T) {
	// A path whose parent is a file, not a directory: MkdirAll cannot make it.
	blocker := filepath.Join(t.TempDir(), "not-a-folder")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := Write(snapshot(), Options{Dir: filepath.Join(blocker, "reports"), Bundle: bundle(t)}); err == nil {
		t.Error("Write succeeded with nowhere to write")
	}
	if err := writeFile(filepath.Join(blocker, "nope", "x.html"), func(*os.File) error { return nil }); err == nil {
		t.Error("writeFile succeeded with nowhere to write")
	}
}

func TestNowFallsBackToTheClockWhenUnset(t *testing.T) {
	var opts Options
	if opts.now().IsZero() {
		t.Error("now returned the zero time with no override set")
	}
}

var errRender = errRenderType{}

type errRenderType struct{}

func (errRenderType) Error() string { return "the render failed" }
