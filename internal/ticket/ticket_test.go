package ticket

import (
	"archive/zip"
	"bytes"
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
		AgentVersion: "test",
		GeneratedAt:  time.Date(2026, 9, 4, 10, 30, 0, 0, time.UTC),
		Host:         platform.Host{OS: platform.Linux, Arch: "amd64"},
		Results: []checks.Result{{
			CheckID:  "disk.volumes",
			Severity: checks.SeverityAttention,
			Summary:  "check.disk.volumes.low",
			Args:     []any{"/", "4.2 GiB"},
		}},
	}
}

func advice() []explain.Advice {
	e := explain.New(nil, nil, platform.Linux)
	return e.ForSnapshot(snapshot())
}

// pngBytes is the smallest thing http.DetectContentType calls an image.
func pngBytes() []byte {
	return []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("payload", 40))
}

func writeTemp(t *testing.T, name string, content []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func build(t *testing.T) *Ticket {
	t.Helper()

	tk, err := New(snapshot(), advice(), "The printer stopped working after lunch.", true)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return tk
}

// entries reads the archive back, which is the only way to assert on what a
// recipient actually gets.
func entries(t *testing.T, tk *Ticket) map[string][]byte {
	t.Helper()

	bundle, err := i18n.Load("en")
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}

	var out bytes.Buffer
	if err := tk.Write(&out, bundle); err != nil {
		t.Fatalf("Write: %v", err)
	}

	archive, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}

	files := make(map[string][]byte)
	for _, f := range archive.File {
		r, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		data := new(bytes.Buffer)
		if _, err := data.ReadFrom(r); err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		_ = r.Close()
		files[f.Name] = data.Bytes()
	}
	return files
}

func TestABundleCarriesEverythingATechnicianNeeds(t *testing.T) {
	tk := build(t)
	if err := tk.Attach(writeTemp(t, "screenshot.png", pngBytes())); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	files := entries(t, tk)

	for _, want := range []string{"ticket.json", "report.html", "attachments/screenshot.png"} {
		if _, ok := files[want]; !ok {
			t.Errorf("the bundle has no %s (got %v)", want, keys(files))
		}
	}

	// The report opens with no network, which is what makes a bundle usable
	// by someone away from the machine.
	rendered := string(files["report.html"])
	for _, forbidden := range []string{"http://", "https://", "<script"} {
		if strings.Contains(rendered, forbidden) {
			t.Errorf("report.html contains %q, so opening it would reach outside the file", forbidden)
		}
	}

	var decoded Ticket
	if err := json.Unmarshal(files["ticket.json"], &decoded); err != nil {
		t.Fatalf("decode ticket.json: %v", err)
	}
	if decoded.Schema != Schema {
		t.Errorf("Schema = %d, want %d", decoded.Schema, Schema)
	}
	if decoded.Description != "The printer stopped working after lunch." {
		t.Errorf("the user's own words did not survive: %q", decoded.Description)
	}
	if len(decoded.Snapshot.Results) != 1 {
		t.Error("the snapshot did not survive")
	}
	if len(decoded.Advice) == 0 {
		t.Error("the offline advice did not travel with the bundle")
	}
	if !decoded.Redacted {
		t.Error("the bundle does not record that it was redacted")
	}
}

// TestTheAttachmentContentIsNotInTheJSON keeps an image out of the structured
// file, where it would bloat every tool that reads it.
func TestTheAttachmentContentIsNotInTheJSON(t *testing.T) {
	tk := build(t)
	if err := tk.Attach(writeTemp(t, "screenshot.png", pngBytes())); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	files := entries(t, tk)
	if bytes.Contains(files["ticket.json"], []byte("payload")) {
		t.Error("the attachment's content was serialised into ticket.json")
	}

	var decoded Ticket
	if err := json.Unmarshal(files["ticket.json"], &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded.Attachments) != 1 || decoded.Attachments[0].Bytes == 0 {
		t.Errorf("the attachment is not described in the JSON: %+v", decoded.Attachments)
	}
}

// TestOnlyImagesAreAccepted is the rule that keeps a support bundle from
// becoming a way to move anything off a machine under a support label.
func TestOnlyImagesAreAccepted(t *testing.T) {
	refused := map[string][]byte{
		"secrets.txt": []byte("BEGIN PRIVATE KEY"),
		"payload.zip": []byte("PK\x03\x04" + strings.Repeat("x", 100)),
		"database.db": []byte("SQLite format 3\x00"),
		"script.sh":   []byte("#!/bin/sh\nrm -rf /\n"),
	}

	for name, content := range refused {
		t.Run(name, func(t *testing.T) {
			tk := build(t)
			if err := tk.Attach(writeTemp(t, name, content)); err == nil {
				t.Errorf("Attach accepted %s", name)
			}
			if len(tk.Attachments) != 0 {
				t.Error("a refused file was still attached")
			}
		})
	}
}

// TestAnImageIsJudgedByItsContentNotItsName: a .png that is really a script
// is not an image, and a .dat that is really a PNG is.
func TestAnImageIsJudgedByItsContentNotItsName(t *testing.T) {
	tk := build(t)
	if err := tk.Attach(writeTemp(t, "screenshot.png", []byte("#!/bin/sh\necho not an image\n"))); err == nil {
		t.Error("a script named .png was accepted")
	}

	tk = build(t)
	if err := tk.Attach(writeTemp(t, "capture.dat", pngBytes())); err != nil {
		t.Errorf("a PNG named .dat was refused: %v", err)
	}
}

func TestAttachmentsAreBoundedInSizeAndNumber(t *testing.T) {
	tk := build(t)

	huge := append(pngBytes(), bytes.Repeat([]byte("x"), MaxAttachmentBytes)...)
	if err := tk.Attach(writeTemp(t, "huge.png", huge)); err == nil {
		t.Error("an oversized attachment was accepted")
	}

	for i := 0; i < MaxAttachments; i++ {
		if err := tk.Attach(writeTemp(t, "shot.png", pngBytes())); err != nil {
			t.Fatalf("attachment %d: %v", i, err)
		}
	}
	if err := tk.Attach(writeTemp(t, "one-too-many.png", pngBytes())); err == nil {
		t.Errorf("a %dth attachment was accepted", MaxAttachments+1)
	}
}

func TestADescriptionLongerThanTheLimitIsRefusedRatherThanTruncated(t *testing.T) {
	// Silently cutting someone's account of their problem in half would be
	// worse than telling them it is too long.
	if _, err := New(snapshot(), nil, strings.Repeat("a", MaxDescription+1), false); err == nil {
		t.Error("an over-long description was accepted")
	}
}

func TestAMissingOrUnreadableFileIsReported(t *testing.T) {
	tk := build(t)

	if err := tk.Attach(filepath.Join(t.TempDir(), "not-there.png")); err == nil {
		t.Error("Attach accepted a path that is not there")
	}
	if err := tk.Attach(t.TempDir()); err == nil {
		t.Error("Attach accepted a folder")
	}
}

// TestTheAttachmentPathIsNotCarried: a path is an identifying detail the user
// did not ask to send. Only the base name travels.
func TestTheAttachmentPathIsNotCarried(t *testing.T) {
	tk := build(t)
	path := writeTemp(t, "screenshot.png", pngBytes())
	if err := tk.Attach(path); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	files := entries(t, tk)
	if bytes.Contains(files["ticket.json"], []byte(filepath.Dir(path))) {
		t.Error("the attachment's directory reached the bundle")
	}
	if tk.Attachments[0].Name != "screenshot.png" {
		t.Errorf("Name = %q, want the base name alone", tk.Attachments[0].Name)
	}
}

func TestSafeName(t *testing.T) {
	cases := map[string]string{
		"screenshot.png":           "screenshot.png",
		"../../etc/passwd":         "passwd",
		"/absolute/path/shot.png":  "shot.png",
		`..\..\windows\system.ini`: "system.ini",
		".hidden":                  "hidden",
		"":                         "attachment",
		"..":                       "attachment",
		// An attachment must never be able to replace the bundle's own files.
		"ticket.json":        "attachment",
		"report.html":        "attachment",
		"weird name (1).png": "weird-name--1-.png",
	}

	for input, want := range cases {
		if got := safeName(input); got != want {
			t.Errorf("safeName(%q) = %q, want %q", input, got, want)
		}
	}

	// Whatever the input, the property that matters is that the result names
	// one file inside attachments/ and cannot climb out of it.
	for input := range cases {
		got := safeName(input)
		if strings.ContainsAny(got, `/\`) {
			t.Errorf("safeName(%q) = %q, which still carries a separator", input, got)
		}
		if got == "" || got == "." || got == ".." || strings.HasPrefix(got, ".") {
			t.Errorf("safeName(%q) = %q, which is not a usable file name", input, got)
		}
	}
}

func TestAnAttachmentCannotOverwriteTheBundlesOwnFiles(t *testing.T) {
	tk := build(t)
	if err := tk.Attach(writeTemp(t, "ticket.json", pngBytes())); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	files := entries(t, tk)
	// The real ticket.json is still the ticket, not the image.
	var decoded Ticket
	if err := json.Unmarshal(files["ticket.json"], &decoded); err != nil {
		t.Fatalf("ticket.json was replaced by the attachment: %v", err)
	}
	if _, ok := files["attachments/attachment"]; !ok {
		t.Errorf("the attachment was not renamed out of the way: %v", keys(files))
	}
}

func TestIDsAreSortableAndDoNotCollide(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 50; i++ {
		tk := build(t)
		if seen[tk.ID] {
			t.Fatalf("ticket ID %q was issued twice", tk.ID)
		}
		seen[tk.ID] = true

		if !strings.HasPrefix(tk.ID, "2026-09-04-1030-") {
			t.Errorf("ID = %q, want it to lead with the snapshot's date", tk.ID)
		}
		if !strings.HasPrefix(tk.Filename(), "supportone-ticket-") || !strings.HasSuffix(tk.Filename(), ".zip") {
			t.Errorf("Filename = %q", tk.Filename())
		}
	}
}

func TestBytesReportsWhatWouldBeSent(t *testing.T) {
	tk := build(t)
	if tk.Bytes() != 0 {
		t.Errorf("Bytes = %d with nothing attached", tk.Bytes())
	}

	if err := tk.Attach(writeTemp(t, "shot.png", pngBytes())); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if tk.Bytes() != len(pngBytes()) {
		t.Errorf("Bytes = %d, want %d", tk.Bytes(), len(pngBytes()))
	}
}

func TestABundleWithoutALanguageStillCarriesTheStructuredTicket(t *testing.T) {
	tk := build(t)

	var out bytes.Buffer
	if err := tk.Write(&out, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}

	archive, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if len(archive.File) != 1 || archive.File[0].Name != "ticket.json" {
		t.Errorf("files = %d, want just ticket.json", len(archive.File))
	}
}

func keys(files map[string][]byte) []string {
	out := make([]string, 0, len(files))
	for name := range files {
		out = append(out, name)
	}
	return out
}
