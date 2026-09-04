// Package schedule produces the monthly client report, and the scheduler entry
// that would produce it every month.
//
// It does not install that entry. Adding a scheduled task is a change to a
// machine, and every change in SupportOne goes through the consent gate as a
// fix with a rollback. What this package does instead is print the exact line
// a person can read, understand and paste — which is also the only version
// that survives someone later asking "what is this thing on my computer, and
// how do I stop it?".
//
// The report itself is written to a folder and sent nowhere. A monthly report
// that mailed itself would be an outbound connection nobody asked for.
package schedule

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/explain"
	"github.com/ZanOzair/SupportOne/internal/i18n"
	"github.com/ZanOzair/SupportOne/internal/report"
)

// Written records what a monthly run produced, so the caller can say where the
// files went rather than only that it worked.
type Written struct {
	// Period is the month the report covers, as a technician would file it.
	Period string `json:"period"`

	HTML string `json:"html"`
	JSON string `json:"json"`

	// Redacted says whether identifying detail was removed.
	Redacted bool `json:"redacted"`
}

// Options is what one monthly run needs.
type Options struct {
	// Dir is the folder the report is written into. It is created if it is
	// not there.
	Dir string

	// Bundle renders the message keys results carry.
	Bundle *i18n.Bundle

	// Advice is the offline explanation, so a report read weeks later still
	// says what its findings mean.
	Advice map[string]explain.Advice

	// Redacted marks the report as one the caller already stripped.
	Redacted bool

	// AuditPath tells the reader where the record of the run lives.
	AuditPath string

	// Now is swappable in tests.
	Now func() time.Time
}

func (o Options) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

// Write produces the month's report in both forms.
//
// Two files, because they answer to two readers: the HTML one is what a client
// is shown, and the JSON one is what a technician's tooling reads. They are
// generated from the same snapshot, so they cannot disagree.
func Write(snap checks.Snapshot, opts Options) (Written, error) {
	if opts.Bundle == nil {
		return Written{}, fmt.Errorf("schedule: a language bundle is required to render message keys")
	}
	if strings.TrimSpace(opts.Dir) == "" {
		return Written{}, fmt.Errorf("schedule: a folder to write the report into is required")
	}
	if err := os.MkdirAll(opts.Dir, 0o700); err != nil {
		return Written{}, fmt.Errorf("schedule: create the report folder: %w", err)
	}

	// The period comes from the snapshot rather than the clock: a report
	// generated late still covers the month it describes.
	stamp := snap.GeneratedAt
	if stamp.IsZero() {
		stamp = opts.now()
	}
	period := stamp.UTC().Format("2006-01")
	base := "supportone-" + period

	written := Written{
		Period:   period,
		HTML:     filepath.Join(opts.Dir, base+".html"),
		JSON:     filepath.Join(opts.Dir, base+".json"),
		Redacted: opts.Redacted,
	}

	if err := writeFile(written.HTML, func(f *os.File) error {
		return report.HTML(f, snap, report.Options{
			Bundle:    opts.Bundle,
			Redacted:  opts.Redacted,
			AuditPath: opts.AuditPath,
			Advice:    opts.Advice,
		})
	}); err != nil {
		return Written{}, err
	}

	if err := writeFile(written.JSON, func(f *os.File) error {
		return report.JSON(f, snap)
	}); err != nil {
		return Written{}, err
	}
	return written, nil
}

// writeFile writes one report atomically at 0600.
//
// A monthly report is about someone's machine and belongs to them, not to
// every account on the host. Writing beside the target and renaming means a
// reader never opens a half-written report, and a run that fails part-way
// leaves last month's intact.
func writeFile(path string, render func(*os.File) error) error {
	dir := filepath.Dir(path)

	temp, err := os.CreateTemp(dir, ".tmp-report-*")
	if err != nil {
		return fmt.Errorf("schedule: create a temporary file: %w", err)
	}
	name := temp.Name()

	if err := render(temp); err != nil {
		_ = temp.Close()
		_ = os.Remove(name)
		return fmt.Errorf("schedule: write %s: %w", filepath.Base(path), err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("schedule: finish writing %s: %w", filepath.Base(path), err)
	}
	if err := os.Chmod(name, 0o600); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("schedule: set permissions on %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("schedule: replace %s: %w", filepath.Base(path), err)
	}
	return nil
}
