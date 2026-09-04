package main

import (
	"bufio"
	"context"
	"fmt"
	"io"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/consent"
	"github.com/ZanOzair/SupportOne/internal/enrol"
	"github.com/ZanOzair/SupportOne/internal/i18n"
	"github.com/ZanOzair/SupportOne/internal/redact"
)

// newEnroller builds the fleet sender. It is off unless the user asked for it
// and named both a server and a name for this machine.
func newEnroller(audit *consent.Log, opts options) *enrol.Enroller {
	return enrol.New(enrol.Config{
		Enabled: opts.report,
		Server:  opts.fleetServer,
		Name:    opts.fleetName,
		Timeout: opts.timeout,
	}, audit, version)
}

// sendReport shows the exact bytes and sends them only if the user says to.
//
// This is the second and last thing the agent does that leaves the machine,
// and it is gated exactly like the first: the payload is shown in full, and
// silence is not consent.
func sendReport(
	ctx context.Context,
	stdin io.Reader,
	w io.Writer,
	bundle *i18n.Bundle,
	audit *consent.Log,
	snap checks.Snapshot,
	opts options,
) error {
	enroller := newEnroller(audit, opts)

	// The terminal path always redacts. Someone reporting a machine into a
	// fleet is not weighing each field, and the protective choice is the one
	// to make on their behalf when they are not asked.
	payload, err := enroller.Prepare(snap, redact.Everything(), redact.CurrentIdentity())
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "\n%s\n", bundle.T("agent.report.heading"))
	fmt.Fprintf(w, "%s\n", bundle.T("agent.report.destination", payload.Host, payload.Name))
	fmt.Fprintf(w, "%s\n", bundle.T("agent.report.size", payload.Bytes))
	fmt.Fprintf(w, "%s\n\n", bundle.T("agent.report.redacted"))
	fmt.Fprintf(w, "%s\n%s\n\n", bundle.T("agent.report.payload"), payload.Body)

	reader := bufio.NewReader(stdin)
	fmt.Fprintf(w, "%s ", bundle.T("agent.report.confirm"))
	if !typed(reader, "send") {
		enroller.Discard(payload.Token)
		fmt.Fprintf(w, "%s\n", bundle.T("agent.report.not_sent"))
		return nil
	}

	result, err := enroller.Send(ctx, payload.Token)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "\n%s\n", bundle.T("agent.report.sent", result.Host))
	fmt.Fprintf(w, "%s\n", bundle.T("agent.report.one_way"))
	return nil
}
