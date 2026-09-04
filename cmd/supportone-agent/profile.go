package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ZanOzair/SupportOne/internal/consent"
	"github.com/ZanOzair/SupportOne/internal/fixes"
	"github.com/ZanOzair/SupportOne/internal/i18n"
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/profile"
)

// errNotConforming is returned when a machine does not meet its profile.
//
// It is an error so that a technician running this over a fleet gets a
// non-zero exit and can act on it. The report is printed first: the exit code
// is a summary of what was already said, never a substitute for saying it.
var errNotConforming = errors.New("this computer does not meet the profile")

// measureProfile checks this machine against a standard a technician wrote.
//
// A profile is data, and it is read as data: it can name a check this build
// does not carry or a fix that does not run here, and both are reported as
// such rather than quietly passing. Nothing in a profile can cause anything to
// be changed — it names fixes to offer, and offering is where it stops.
func measureProfile(ctx context.Context, w io.Writer, bundle *i18n.Bundle, audit *consent.Log, host platform.Host, opts options) error {
	file, err := os.Open(opts.profile) // #nosec G304 -- the profile the technician asked to measure against.
	if err != nil {
		return fmt.Errorf("open the profile: %w", err)
	}
	defer func() { _ = file.Close() }()

	standard, err := profile.Load(file)
	if err != nil {
		return err
	}

	snap := takeSnapshot(ctx, audit, host, opts)
	measured := standard.Measure(snap, fixes.Default, host.OS)

	_ = audit.Append(consent.Event{
		Type:    consent.EventCheckRun,
		Subject: "profile " + measured.Profile,
		Fields: map[string]string{
			"met":     fmt.Sprint(measured.Met),
			"unmet":   fmt.Sprint(measured.Unmet),
			"unknown": fmt.Sprint(measured.Unknown),
			"missing": fmt.Sprint(measured.Missing),
		},
	})

	if opts.json {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(measured); err != nil {
			return err
		}
	} else {
		writeProfileReport(w, bundle, measured)
	}

	if !measured.Conforms() {
		return errNotConforming
	}
	return nil
}

// writeProfileReport prints the findings worst-first.
func writeProfileReport(w io.Writer, bundle *i18n.Bundle, measured profile.Report) {
	fmt.Fprintf(w, "%s\n\n", bundle.T("agent.profile.heading", measured.Profile))

	for _, finding := range profile.Ordered(measured.Findings) {
		fmt.Fprintf(w, "%-9s %s\n", stateLabel(bundle, finding.State), finding.Check)

		switch finding.State {
		case profile.StateMissing:
			fmt.Fprintf(w, "          %s\n", bundle.T("agent.profile.not_here"))
		case profile.StateUnknown:
			fmt.Fprintf(w, "          %s\n", bundle.T("agent.profile.no_answer"))
		default:
			if finding.Summary != "" {
				fmt.Fprintf(w, "          %s\n", bundle.T(finding.Summary, finding.Args...))
			}
		}

		if finding.State != profile.StateMet {
			fmt.Fprintf(w, "          %s\n", bundle.T("agent.profile.allowed", string(finding.Worst)))
		}
		if finding.Why != "" {
			// The technician's own note, printed as written: it is not a
			// message key and this build does not translate it.
			fmt.Fprintf(w, "          %s\n", finding.Why)
		}
		for _, id := range finding.Offer {
			fmt.Fprintf(w, "          %s\n", bundle.T("agent.profile.offer", id))
		}
	}

	fmt.Fprintf(w, "\n%s\n", bundle.T("agent.profile.tally",
		measured.Met, measured.Unmet, measured.Unknown, measured.Missing))

	if measured.Conforms() {
		fmt.Fprintf(w, "%s\n", bundle.T("agent.profile.conforms"))
		return
	}
	fmt.Fprintf(w, "%s\n", bundle.T("agent.profile.does_not_conform"))
	if measured.Unknown > 0 || measured.Missing > 0 {
		fmt.Fprintf(w, "%s\n", bundle.T("agent.profile.unanswered_counts_against"))
	}
}

func stateLabel(bundle *i18n.Bundle, state profile.State) string {
	switch state {
	case profile.StateMet:
		return bundle.T("agent.profile.state.met")
	case profile.StateUnmet:
		return bundle.T("agent.profile.state.unmet")
	case profile.StateUnknown:
		return bundle.T("agent.profile.state.unknown")
	default:
		return bundle.T("agent.profile.state.missing")
	}
}
