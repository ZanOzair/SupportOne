// Package report renders a snapshot for a person to read.
//
// The HTML report is a single self-contained file: no scripts, no fonts, no
// stylesheets fetched from anywhere. It can be opened from a USB stick on a
// machine with no network, and emailed to a technician without dragging
// anything else along.
package report

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/explain"
	"github.com/ZanOzair/SupportOne/internal/i18n"
)

//go:embed templates/*.gohtml
var templateFS embed.FS

var htmlTemplate = template.Must(template.ParseFS(templateFS, "templates/*.gohtml"))

// Options carry what the renderer needs beyond the snapshot itself.
type Options struct {
	// Bundle resolves the message keys results carry into the reader's
	// language.
	Bundle *i18n.Bundle

	// Redacted marks the report as one the user has already stripped, so a
	// reader knows the blanks are deliberate.
	Redacted bool

	// AuditPath tells the reader where the record of this run lives.
	AuditPath string

	// Advice is the offline explanation of each finding, keyed by check ID.
	// A report without it still renders every verdict; with it, the reader
	// gets the same plain-language cause and next steps the interface shows,
	// which is what makes a saved report useful to someone away from the
	// machine.
	Advice map[string]explain.Advice
}

// severityOrder puts the results that matter first. Within a severity the
// order stays alphabetical by check ID, so two runs of the same machine produce
// comparable reports.
var severityOrder = map[checks.Severity]int{
	checks.SeverityUrgent:    0,
	checks.SeverityAttention: 1,
	checks.SeverityUnknown:   2,
	checks.SeverityOK:        3,
}

// patchRow is one applied update as the report lists it.
type patchRow struct {
	ID      string
	Title   string
	Applied string
}

type page struct {
	Title      string
	Generated  string
	Version    string
	OS         string
	Arch       string
	Unsigned   bool
	Redacted   bool
	AuditPath  string
	Counts     []count
	Results    []result
	Skipped    []string
	Strings    map[string]string
	NoFindings bool

	// Patches is what the machine records having applied. It is rendered as
	// its own dated table rather than buried in one check's evidence,
	// because a monthly patch statement is a thing a technician is asked for
	// on its own.
	Patches       []patchRow
	PatchSource   string
	PatchesCapped bool
}

type count struct {
	Severity string
	Label    string
	Value    int
}

type result struct {
	ID       string
	Severity string
	Label    string
	Summary  string
	Error    string
	Fields   []field
	Evidence string

	// Cause and Steps are the offline explanation, already rendered into the
	// reader's language.
	Cause string
	Steps []string
}

type field struct {
	Name  string
	Value string
}

// HTML writes a self-contained report.
func HTML(w io.Writer, snap checks.Snapshot, opts Options) error {
	if opts.Bundle == nil {
		return fmt.Errorf("report: a language bundle is required to render message keys")
	}
	return htmlTemplate.ExecuteTemplate(w, "report.gohtml", buildPage(snap, opts))
}

// JSON writes the snapshot as it stands, which is exactly what a technician's
// tooling consumes and exactly what the user reviewed on screen.
func JSON(w io.Writer, snap checks.Snapshot) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(snap)
}

func buildPage(snap checks.Snapshot, opts Options) page {
	b := opts.Bundle

	p := page{
		Title:     b.T("report.title"),
		Generated: snap.GeneratedAt.UTC().Format("2006-01-02 15:04 MST"),
		Version:   snap.AgentVersion,
		OS:        snap.Host.OS.Display(),
		Arch:      snap.Host.Arch,
		Unsigned:  snap.AgentVersion == "" || snap.AgentVersion == "dev",
		Redacted:  opts.Redacted,
		AuditPath: opts.AuditPath,
		Skipped:   snap.SkippedAdmin,
		Strings:   reportStrings(b),
	}

	counts := snap.Counts()
	for _, severity := range []checks.Severity{
		checks.SeverityUrgent, checks.SeverityAttention, checks.SeverityUnknown, checks.SeverityOK,
	} {
		p.Counts = append(p.Counts, count{
			Severity: string(severity),
			Label:    b.T("severity." + string(severity)),
			Value:    counts[severity],
		})
	}
	// Nothing wrong is a result worth stating plainly rather than leaving the
	// reader to infer it from an absence.
	p.NoFindings = counts[checks.SeverityUrgent] == 0 && counts[checks.SeverityAttention] == 0

	ordered := make([]checks.Result, len(snap.Results))
	copy(ordered, snap.Results)
	sort.SliceStable(ordered, func(i, j int) bool {
		if severityOrder[ordered[i].Severity] != severityOrder[ordered[j].Severity] {
			return severityOrder[ordered[i].Severity] < severityOrder[ordered[j].Severity]
		}
		return ordered[i].CheckID < ordered[j].CheckID
	})

	for _, res := range ordered {
		fields, evidence := describeDetail(res.Detail)
		row := result{
			ID:       res.CheckID,
			Severity: string(res.Severity),
			Label:    b.T("severity." + string(res.Severity)),
			Summary:  b.T(res.Summary, res.Args...),
			Error:    res.Err,
			Fields:   fields,
			Evidence: evidence,
		}

		if advice, ok := opts.Advice[res.CheckID]; ok {
			row.Cause = b.T(advice.Cause)
			for _, step := range advice.Steps {
				row.Steps = append(row.Steps, b.T(step))
			}
		}
		p.Results = append(p.Results, row)
	}

	p.Patches, p.PatchSource, p.PatchesCapped = patchTable(snap, b)
	return p
}

// patchTable pulls the applied-update list out of the patch check's evidence.
//
// It reads the same detail the JSON report carries rather than re-collecting
// anything, so the table and the check can never disagree, and a snapshot the
// user redacted stays redacted here too.
func patchTable(snap checks.Snapshot, b *i18n.Bundle) (rows []patchRow, source string, capped bool) {
	for _, res := range snap.Results {
		if res.CheckID != "updates.installed" {
			continue
		}

		if named, ok := res.Detail["source"].(string); ok {
			source = b.T("report.patch_source", named)
		}
		recorded, _ := res.Detail["recorded"].(float64)

		entries, ok := res.Detail["patches"].([]any)
		if !ok {
			// The snapshot came from JSON, where the list is []any, or from a
			// live run, where it is the typed slice. Handle both rather than
			// silently rendering nothing.
			if typed, isTyped := res.Detail["patches"].([]map[string]any); isTyped {
				entries = make([]any, 0, len(typed))
				for _, e := range typed {
					entries = append(entries, e)
				}
			}
		}

		for _, entry := range entries {
			fields, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			rows = append(rows, patchRow{
				ID:      text(fields["id"]),
				Title:   text(fields["title"]),
				Applied: text(fields["applied"]),
			})
		}

		capped = int(recorded) > len(rows)
		return rows, source, capped
	}
	return nil, "", false
}

// text renders an evidence field that may be absent.
func text(value any) string {
	s, _ := value.(string)
	return s
}

// describeDetail splits a check's evidence into scalar fields, which read well
// as a table, and everything else, which is shown as the JSON it is rather than
// flattened into something that looks tidier than the truth.
func describeDetail(detail map[string]any) ([]field, string) {
	if len(detail) == 0 {
		return nil, ""
	}

	var fields []field
	nested := make(map[string]any)
	for key, value := range detail {
		switch v := value.(type) {
		case string:
			fields = append(fields, field{Name: key, Value: v})
		case bool:
			fields = append(fields, field{Name: key, Value: fmt.Sprint(v)})
		case int, int32, int64, uint, uint32, uint64, float32, float64:
			fields = append(fields, field{Name: key, Value: fmt.Sprint(v)})
		default:
			nested[key] = value
		}
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })

	if len(nested) == 0 {
		return fields, ""
	}
	encoded, err := json.MarshalIndent(nested, "", "  ")
	if err != nil {
		return fields, ""
	}
	return fields, string(encoded)
}

// reportStrings collects the fixed labels the template needs, so the template
// itself holds no English. Keys are stored with underscores because a Go
// template cannot address a map key containing a dot with field syntax.
func reportStrings(b *i18n.Bundle) map[string]string {
	keys := []string{
		"report.title", "report.subtitle", "report.machine", "report.generated",
		"report.agent", "report.unsigned", "report.summary", "report.no_findings",
		"report.evidence", "report.skipped", "report.skipped_note", "report.audit",
		"report.about", "report.about_body", "report.redacted", "report.checked",
		"report.what_this_means", "report.what_to_do",
		"report.patches", "report.patches_note",
		"report.patch_id", "report.patch_title", "report.patch_applied",
	}
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[strings.ReplaceAll(key, ".", "_")] = b.T(key)
	}
	return out
}

// Filename returns a stable, sortable name for a saved report.
func Filename(snap checks.Snapshot, extension string) string {
	stamp := snap.GeneratedAt.UTC().Format("2006-01-02-1504")
	if snap.GeneratedAt.IsZero() {
		stamp = time.Now().UTC().Format("2006-01-02-1504")
	}
	return fmt.Sprintf("supportone-%s.%s", stamp, strings.TrimPrefix(extension, "."))
}
