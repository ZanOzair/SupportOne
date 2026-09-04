package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/ZanOzair/SupportOne/internal/consent"
	"github.com/ZanOzair/SupportOne/internal/explain"
	"github.com/ZanOzair/SupportOne/internal/i18n"
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/redact"
	"github.com/ZanOzair/SupportOne/internal/schedule"
)

// writeMonthly produces the month's client report and saves it locally.
//
// It sends nothing. A monthly report that mailed itself would be an outbound
// connection nobody asked for, and this is the one command most likely to be
// run unattended by a scheduler — which is exactly when nobody is there to be
// asked.
func writeMonthly(ctx context.Context, w io.Writer, bundle *i18n.Bundle, audit *consent.Log, host platform.Host, opts options) error {
	snap := takeSnapshot(ctx, audit, host, opts)

	// A report that runs unattended redacts fully. Nobody is sitting there
	// weighing each field, and the protective choice is the one to make when
	// the question cannot be asked.
	policy := redact.Everything()
	redacted, err := policy.Snapshot(snap, redact.CurrentIdentity())
	if err != nil {
		return err
	}

	advice := make(map[string]explain.Advice)
	for _, a := range newExplainer(host).ForSnapshot(redacted) {
		advice[a.CheckID] = a
	}

	written, err := schedule.Write(redacted, schedule.Options{
		Dir:       opts.monthly,
		Bundle:    bundle,
		Advice:    advice,
		Redacted:  true,
		AuditPath: audit.Path(),
	})
	if err != nil {
		return err
	}

	_ = audit.Append(consent.Event{
		Type:    consent.EventDataSent,
		Subject: "monthly report " + written.Period,
		Fields: map[string]string{
			"destination": "local file",
			"redacted":    "true",
		},
	})

	fmt.Fprintf(w, "%s\n", bundle.T("agent.monthly.written", written.Period))
	fmt.Fprintf(w, "  %s\n  %s\n", written.HTML, written.JSON)
	fmt.Fprintf(w, "%s\n", bundle.T("agent.monthly.not_sent"))
	return nil
}

// printSchedule prints the scheduler entry and installs nothing.
//
// Adding a scheduled task is a change to a machine, and every change here goes
// through the consent gate as a fix with a rollback. Printing it is also the
// version that survives someone asking, a year later, what this thing on their
// computer is and how to stop it.
func printSchedule(w io.Writer, bundle *i18n.Bundle, host platform.Host, opts options) error {
	binary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("work out where this program lives: %w", err)
	}

	entry, err := schedule.EntryFor(host.OS, binary, opts.scheduleDir)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "%s\n\n", bundle.T("agent.schedule.heading", entry.Mechanism))
	if entry.Where != "" {
		fmt.Fprintf(w, "%s\n\n", bundle.T("agent.schedule.where", entry.Where))
	}
	fmt.Fprintf(w, "%s\n\n", entry.Command)
	fmt.Fprintf(w, "%s %s\n\n", bundle.T("agent.schedule.undo"), entry.Undo)
	fmt.Fprintf(w, "%s\n", bundle.T("agent.schedule.not_installed"))
	return nil
}
