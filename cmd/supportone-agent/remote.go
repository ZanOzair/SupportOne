package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/ZanOzair/SupportOne/internal/consent"
	"github.com/ZanOzair/SupportOne/internal/i18n"
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/remote"
)

// newRemote builds the consent wrapper for a remote-help session.
func newRemote(audit *consent.Log, host platform.Host) *remote.Wrapper {
	return remote.New(audit, host.OS)
}

// listRemoteTools reports which remote-help programs are already on this
// machine. It offers to install none of them.
func listRemoteTools(w io.Writer, bundle *i18n.Bundle, audit *consent.Log, host platform.Host) error {
	tools := newRemote(audit, host).Tools()
	if len(tools) == 0 {
		fmt.Fprintf(w, "%s\n", bundle.T("agent.remote.none_known"))
		return nil
	}

	for _, tool := range tools {
		if tool.Installed {
			fmt.Fprintf(w, "%-16s %s\n", tool.ID, bundle.T("agent.remote.found", tool.Name, tool.Path))
			continue
		}
		fmt.Fprintf(w, "%-16s %s\n", tool.ID, bundle.T("agent.remote.not_found", tool.Name))
	}
	fmt.Fprintf(w, "\n%s\n", bundle.T("agent.remote.no_install"))
	return nil
}

// startRemote shows what a remote session lets someone do, waits for the user
// to agree in those terms, and only then starts the tool.
//
// The whole command is the consent step. SupportOne is not part of the session
// that follows and cannot be: it says so here rather than leaving the user to
// assume otherwise.
func startRemote(ctx context.Context, stdin io.Reader, w io.Writer, bundle *i18n.Bundle, audit *consent.Log, host platform.Host, opts options) error {
	wrapper := newRemote(audit, host)

	plan, err := wrapper.Plan(opts.remote, opts.remoteTool)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "%s\n\n", bundle.T("agent.remote.heading", plan.Technician))
	for _, key := range plan.Consequences {
		fmt.Fprintf(w, "  - %s\n", bundle.T(key))
	}
	if plan.Tool.ID != "" {
		fmt.Fprintf(w, "\n%s\n", bundle.T("agent.remote.will_start", plan.Tool.Name, plan.Tool.Path))
	} else {
		fmt.Fprintf(w, "\n%s\n", bundle.T("agent.remote.no_tool"))
	}

	if opts.dryRun {
		fmt.Fprintf(w, "\n%s\n", bundle.T("agent.dry_run.active"))
		return nil
	}

	reader := bufio.NewReader(stdin)

	fmt.Fprintf(w, "\n%s ", bundle.T("agent.remote.confirm"))
	if !typed(reader, "allow") {
		wrapper.Decline()
		fmt.Fprintf(w, "%s\n", bundle.T("agent.remote.aborted"))
		return nil
	}

	session, err := wrapper.Start(ctx, remote.Confirmation{
		Token: plan.Token,
		// The list above is what was printed, in the order it was printed.
		Acknowledged: plan.Consequences,
	})
	if err != nil {
		return err
	}

	if session.Launched {
		fmt.Fprintf(w, "\n%s\n", bundle.T("agent.remote.started", plan.Tool.Name))
	} else {
		fmt.Fprintf(w, "\n%s\n", bundle.T("agent.remote.start_it_yourself"))
	}
	fmt.Fprintf(w, "%s\n", bundle.T("agent.remote.not_watching"))

	// Waiting here is what turns the audit log's two lines into a length. The
	// process ending for any other reason leaves the session open in the
	// record, which is the honest state: nobody told SupportOne it was over.
	fmt.Fprintf(w, "\n%s ", bundle.T("agent.remote.press_enter"))
	if _, err := reader.ReadString('\n'); err != nil && err != io.EOF {
		return fmt.Errorf("wait for you to say the session is over: %w", err)
	}

	ended, err := wrapper.End()
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "\n%s\n", bundle.T("agent.remote.ended", ended.Duration().Round(time.Second).String()))
	fmt.Fprintf(w, "%s\n", bundle.T(remote.KeyCloseAnything))
	return nil
}
