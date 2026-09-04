package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZanOzair/SupportOne/internal/consent"
	"github.com/ZanOzair/SupportOne/internal/i18n"
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/redact"
	"github.com/ZanOzair/SupportOne/internal/ticket"
)

// writeTicket builds a support bundle and saves it beside the user, not to
// anyone else.
//
// Writing a file is not sending one. This produces something the user can
// attach to an email, hand over on a stick, or delete — and deciding which is
// theirs, which is why nothing here opens a connection.
func writeTicket(ctx context.Context, w io.Writer, bundle *i18n.Bundle, audit *consent.Log, host platform.Host, opts options) error {
	snap := takeSnapshot(ctx, audit, host, opts)

	// The terminal path redacts fully. A bundle is built to be handed to
	// someone else, and the protective choice is the one to make on the
	// user's behalf when they are not being asked field by field.
	policy := redact.Everything()
	redacted, err := policy.Snapshot(snap, redact.CurrentIdentity())
	if err != nil {
		return err
	}

	tk, err := ticket.New(redacted, newExplainer(host).ForSnapshot(redacted), opts.describe, true)
	if err != nil {
		return err
	}

	for _, path := range opts.attachments() {
		if err := tk.Attach(path); err != nil {
			return err
		}
	}

	path := opts.ticket
	if isDirectory(path) {
		path = filepath.Join(path, tk.Filename())
	}

	file, err := os.Create(path) // #nosec G304 -- the path the user asked to write to.
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	if err := tk.Write(file, bundle); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("finish writing %s: %w", path, err)
	}

	// Saving a file is recorded the same way saving a report is: it is the
	// user's own machine, and the log is their record of what left it.
	_ = audit.Append(consent.Event{
		Type:    consent.EventDataSent,
		Subject: tk.Filename(),
		Fields: map[string]string{
			"destination": "local file",
			"redacted":    "true",
			"attachments": fmt.Sprint(len(tk.Attachments)),
		},
	})

	fmt.Fprintf(w, "%s\n", bundle.T("agent.ticket.written", path))
	fmt.Fprintf(w, "%s\n", bundle.T("agent.ticket.contents", len(tk.Attachments)))
	fmt.Fprintf(w, "%s\n", bundle.T("agent.ticket.not_sent"))
	return nil
}

// attachments splits the comma-separated list the user gave.
func (o options) attachments() []string {
	if strings.TrimSpace(o.attach) == "" {
		return nil
	}

	var out []string
	for _, path := range strings.Split(o.attach, ",") {
		if trimmed := strings.TrimSpace(path); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
