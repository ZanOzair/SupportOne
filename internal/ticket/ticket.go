// Package ticket packages what a technician needs into one file.
//
// A support conversation usually starts with a person describing a problem in
// their own words and a screenshot of it. What is missing is everything the
// machine could have said for itself, and what is usually oversupplied is
// whatever happened to be on screen. A ticket here carries the description,
// the redacted snapshot, the offline advice, and an image the user chose — in
// one file they can attach to an email or hand over on a stick.
//
// # SupportOne does not take the screenshot
//
// It has no screen-capture code, deliberately. A screenshot cannot be redacted
// by field: it captures whatever happened to be visible, which is routinely a
// document, a password manager, or someone else's message. Redaction can strip
// a hostname out of a JSON field; it cannot strip a bank balance out of a
// picture of a bank balance.
//
// So the user picks the file. By the time they do, they have already decided
// what is in it — which is a better guarantee than any capture-then-review
// flow, and it means this binary has no capability to abuse.
package ticket

import (
	"archive/zip"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/explain"
	"github.com/ZanOzair/SupportOne/internal/i18n"
	"github.com/ZanOzair/SupportOne/internal/report"
)

// Limits. Each is a refusal rather than a truncation: a ticket that silently
// dropped half an attachment would be worse than one that would not build.
const (
	// MaxAttachmentBytes bounds one file. A screenshot is well under this;
	// a video is not a screenshot.
	MaxAttachmentBytes = 10 << 20

	// MaxAttachments bounds how many files one ticket carries.
	MaxAttachments = 4

	// MaxDescription bounds the user's own text.
	MaxDescription = 8000
)

// Schema is the version of the ticket.json format, so a technician's tooling
// can rely on it.
const Schema = 1

// Attachment is one file the user chose to include.
type Attachment struct {
	// Name is the file's base name, with any directory stripped. The path it
	// came from is not carried: a path is an identifying detail the user did
	// not ask to send.
	Name string `json:"name"`

	// MediaType is sniffed from the content, not taken from the extension,
	// so the recipient is told what the file actually is.
	MediaType string `json:"media_type"`

	Bytes int `json:"bytes"`

	// content is what gets written into the archive. It is not serialised
	// into ticket.json; the file itself lives beside it.
	content []byte
}

// Ticket is what a technician receives.
type Ticket struct {
	Schema  int       `json:"schema"`
	ID      string    `json:"id"`
	Created time.Time `json:"created"`

	// Description is the user's own words about the problem. It is carried
	// verbatim: this is the one part of the bundle SupportOne did not write.
	Description string `json:"description"`

	// Snapshot is the machine's own account of itself, already redacted by
	// the policy the user chose.
	Snapshot checks.Snapshot `json:"snapshot"`

	// Advice is the offline explanation of each finding, so the technician
	// reads the same thing the user was shown.
	Advice []explain.Advice `json:"advice,omitempty"`

	// Redacted says whether anything was removed, so a reader knows the
	// blanks are deliberate.
	Redacted bool `json:"redacted"`

	Attachments []Attachment `json:"attachments,omitempty"`
}

// New starts a ticket from a snapshot the caller has already redacted.
//
// Redaction is the caller's decision and is applied before this is called, so
// nothing here can un-redact anything: the ticket carries what it was given.
func New(snap checks.Snapshot, advice []explain.Advice, description string, redacted bool) (*Ticket, error) {
	if len(description) > MaxDescription {
		return nil, fmt.Errorf("ticket: the description is %d characters, and %d is the limit", len(description), MaxDescription)
	}

	id, err := newID(snap.GeneratedAt)
	if err != nil {
		return nil, err
	}

	return &Ticket{
		Schema:      Schema,
		ID:          id,
		Created:     time.Now().UTC(),
		Description: strings.TrimSpace(description),
		Snapshot:    snap,
		Advice:      advice,
		Redacted:    redacted,
	}, nil
}

// Attach adds a file the user chose.
//
// The media type is sniffed from the content rather than trusted from the
// extension, and only images are accepted: a ticket is a description and a
// picture of the problem, and accepting arbitrary files would make it a way to
// move anything off a machine under a support label.
func (t *Ticket) Attach(path string) error {
	if len(t.Attachments) >= MaxAttachments {
		return fmt.Errorf("ticket: a ticket carries at most %d attachments", MaxAttachments)
	}

	// #nosec G304 -- the path is one the user named on their own machine to
	// attach to their own ticket.
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("ticket: open %s: %w", filepath.Base(path), err)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("ticket: read %s: %w", filepath.Base(path), err)
	}
	if info.IsDir() {
		return fmt.Errorf("ticket: %s is a folder, not a file", filepath.Base(path))
	}
	if info.Size() > MaxAttachmentBytes {
		return fmt.Errorf("ticket: %s is %d bytes, and %d is the limit for one attachment",
			filepath.Base(path), info.Size(), MaxAttachmentBytes)
	}

	content, err := io.ReadAll(io.LimitReader(file, MaxAttachmentBytes+1))
	if err != nil {
		return fmt.Errorf("ticket: read %s: %w", filepath.Base(path), err)
	}
	if len(content) > MaxAttachmentBytes {
		return fmt.Errorf("ticket: %s is larger than the %d byte limit", filepath.Base(path), MaxAttachmentBytes)
	}

	mediaType := http.DetectContentType(content)
	if !strings.HasPrefix(mediaType, "image/") {
		return fmt.Errorf("ticket: %s is %s; a ticket carries images, not arbitrary files",
			filepath.Base(path), mediaType)
	}

	t.Attachments = append(t.Attachments, Attachment{
		Name:      safeName(filepath.Base(path)),
		MediaType: mediaType,
		Bytes:     len(content),
		content:   content,
	})
	return nil
}

// Write builds the archive.
//
// Three files go in: the structured ticket a tool can read, the same report a
// person can open in a browser with no network, and whatever the user
// attached. Nothing is fetched while writing it.
func (t *Ticket) Write(w io.Writer, bundle *i18n.Bundle) error {
	archive := zip.NewWriter(w)

	if err := t.writeJSON(archive); err != nil {
		return err
	}
	if err := t.writeReport(archive, bundle); err != nil {
		return err
	}
	if err := t.writeAttachments(archive); err != nil {
		return err
	}

	if err := archive.Close(); err != nil {
		return fmt.Errorf("ticket: finish the archive: %w", err)
	}
	return nil
}

func (t *Ticket) writeJSON(archive *zip.Writer) error {
	entry, err := archive.Create("ticket.json")
	if err != nil {
		return fmt.Errorf("ticket: add ticket.json: %w", err)
	}

	encoder := json.NewEncoder(entry)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(t); err != nil {
		return fmt.Errorf("ticket: write ticket.json: %w", err)
	}
	return nil
}

func (t *Ticket) writeReport(archive *zip.Writer, bundle *i18n.Bundle) error {
	if bundle == nil {
		return nil
	}

	entry, err := archive.Create("report.html")
	if err != nil {
		return fmt.Errorf("ticket: add report.html: %w", err)
	}

	advice := make(map[string]explain.Advice, len(t.Advice))
	for _, a := range t.Advice {
		advice[a.CheckID] = a
	}

	opts := report.Options{Bundle: bundle, Redacted: t.Redacted, Advice: advice}
	if err := report.HTML(entry, t.Snapshot, opts); err != nil {
		return fmt.Errorf("ticket: write report.html: %w", err)
	}
	return nil
}

func (t *Ticket) writeAttachments(archive *zip.Writer) error {
	for _, a := range t.Attachments {
		entry, err := archive.Create("attachments/" + a.Name)
		if err != nil {
			return fmt.Errorf("ticket: add %s: %w", a.Name, err)
		}
		if _, err := entry.Write(a.content); err != nil {
			return fmt.Errorf("ticket: write %s: %w", a.Name, err)
		}
	}
	return nil
}

// Filename returns a stable, sortable name for the bundle.
func (t *Ticket) Filename() string {
	return fmt.Sprintf("supportone-ticket-%s.zip", t.ID)
}

// Bytes reports how large the ticket's contents are, so the user can be told
// before they attach it to anything.
func (t *Ticket) Bytes() int {
	total := 0
	for _, a := range t.Attachments {
		total += a.Bytes
	}
	return total
}

// newID builds an identifier that sorts by time and does not collide. The
// date makes it readable in a mailbox; the random tail keeps two machines
// filing at the same moment apart.
func newID(generated time.Time) (string, error) {
	if generated.IsZero() {
		generated = time.Now()
	}

	buf := make([]byte, 5)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("ticket: generate ticket ID: %w", err)
	}
	tail := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf))

	return generated.UTC().Format("2006-01-02-1504") + "-" + tail, nil
}

// safeName reduces a file name to something safe to write into an archive and
// safe for a recipient to extract: no directories, no traversal, no leading
// dot, and nothing that would overwrite the bundle's own files.
//
// Backslashes are treated as separators whatever the agent is running on. The
// machine that built the bundle and the machine that opens it are often not
// the same platform, and a Windows path handled on Linux would otherwise keep
// its separators all the way into the archive.
func safeName(name string) string {
	name = filepath.Base(filepath.Clean(strings.ReplaceAll(name, `\`, "/")))
	name = strings.TrimLeft(name, ".")

	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '.' || r == '-' || r == '_':
			return r
		default:
			return '-'
		}
	}, name)

	if name == "" || name == "ticket.json" || name == "report.html" {
		return "attachment"
	}
	return name
}
