package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/ZanOzair/SupportOne/internal/consent"
	"github.com/ZanOzair/SupportOne/internal/fixes"
	"github.com/ZanOzair/SupportOne/internal/i18n"
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/remediate"
	"github.com/ZanOzair/SupportOne/internal/wizard"
)

func listFixes(w io.Writer, bundle *i18n.Bundle, host platform.Host) error {
	available := fixes.Default.ForPlatform(host.OS)
	if len(available) == 0 {
		fmt.Fprintln(w, bundle.T("agent.fixes.none"))
		return nil
	}

	fmt.Fprintln(w, bundle.T("agent.fixes.available", len(available), host.OS.Display()))
	for _, f := range available {
		fmt.Fprintf(w, "\n  %s\n    %s\n", f.ID(), bundle.T(f.Describe().Summary))
		if f.RequiresAdmin() {
			fmt.Fprintf(w, "    %s\n", bundle.T("agent.checks.requires_admin"))
		}
		if !f.Reversible() {
			fmt.Fprintf(w, "    %s\n", bundle.T("agent.fixes.not_reversible"))
		}
	}
	return nil
}

func listWizards(w io.Writer, bundle *i18n.Bundle, host platform.Host) error {
	available := wizard.Default.ForPlatform(host.OS)
	if len(available) == 0 {
		fmt.Fprintln(w, bundle.T("agent.wizards.none"))
		return nil
	}

	fmt.Fprintln(w, bundle.T("agent.wizards.available", len(available), host.OS.Display()))
	for _, wz := range available {
		fmt.Fprintf(w, "\n  %s\n    %s\n", wz.ID, bundle.T(wz.Title))
	}
	return nil
}

// applyFix describes one repair and applies it only if the user types its ID
// back. Printing what will change and then acting on a bare "y" would be a
// confirmation of having read nothing.
func applyFix(ctx context.Context, stdin io.Reader, w io.Writer, bundle *i18n.Bundle, audit *consent.Log, host platform.Host, opts options) error {
	applier := newApplier(audit, host, opts)

	plan, err := applier.Plan(ctx, opts.fix)
	if err != nil {
		return err
	}
	writePlan(w, bundle, plan)

	if plan.Blocked != "" {
		fmt.Fprintf(w, "\n%s\n", bundle.T("agent.fix.blocked", bundle.T(plan.Blocked)))
		return nil
	}
	if plan.RequiresAdmin && !plan.Elevated {
		fmt.Fprintf(w, "\n%s\n", bundle.T("agent.fix.needs_admin"))
		return nil
	}
	if plan.DryRun {
		fmt.Fprintf(w, "\n%s\n", bundle.T("agent.dry_run.active"))
		return nil
	}

	reader := bufio.NewReader(stdin)

	fmt.Fprintf(w, "\n%s ", bundle.T("agent.fix.confirm", plan.FixID))
	if !typed(reader, plan.FixID) {
		fmt.Fprintf(w, "%s\n", bundle.T("agent.fix.aborted"))
		return nil
	}

	confirmation := remediate.Confirmation{
		Token:        plan.Token,
		Acknowledged: plan.Explanation.Changes,
	}

	if !plan.Restore.Available {
		// The user has already been told there is no restore point. Going
		// ahead is a second, separate decision.
		fmt.Fprintf(w, "%s ", bundle.T("agent.fix.confirm_no_restore"))
		if !typed(reader, "yes") {
			fmt.Fprintf(w, "%s\n", bundle.T("agent.fix.aborted"))
			return nil
		}
		confirmation.AcceptWithoutRestorePoint = true
	}

	result, err := applier.Apply(ctx, confirmation)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "\n%s\n", bundle.T(result.Outcome.Detail, result.Outcome.DetailArgs...))
	if result.RestorePoint != nil {
		fmt.Fprintf(w, "%s\n", bundle.T("agent.fix.restore_point", result.RestorePoint.Kind))
	}
	if !result.Reversible {
		return nil
	}

	// The record of what was applied lives in this process, so the offer to
	// undo it is made here and now rather than pointed at a later command
	// that would have nothing to undo.
	fmt.Fprintf(w, "\n%s ", bundle.T("agent.fix.undo_now"))
	if !typed(reader, "undo") {
		return nil
	}
	if err := applier.Rollback(ctx, plan.FixID); err != nil {
		return err
	}
	fmt.Fprintf(w, "%s\n", bundle.T("agent.fix.undone"))
	return nil
}

// writePlan prints exactly what the fix would do, before anything happens.
func writePlan(w io.Writer, bundle *i18n.Bundle, plan remediate.Plan) {
	fmt.Fprintf(w, "%s\n\n%s\n\n%s\n", plan.FixID, bundle.T(plan.Explanation.Summary), bundle.T("agent.fix.changes"))
	for _, change := range plan.Explanation.Changes {
		fmt.Fprintf(w, "  - %s\n", bundle.T(change))
	}

	fmt.Fprintf(w, "\n%s %s\n", bundle.T("agent.fix.undo"), bundle.T(plan.Explanation.Undo))
	if plan.Restore.Available {
		fmt.Fprintf(w, "%s\n", bundle.T("agent.fix.restore_available", plan.Restore.Kind))
	} else {
		fmt.Fprintf(w, "%s\n", bundle.T("agent.fix.restore_unavailable", bundle.T(plan.Restore.Reason)))
	}
}

// runWizard walks the questions one at a time, stopping at each one that finds
// something and asking before it changes anything.
func runWizard(ctx context.Context, stdin io.Reader, w io.Writer, bundle *i18n.Bundle, audit *consent.Log, host platform.Host, opts options) error {
	wz, ok := wizard.Default.Get(opts.wizard)
	if !ok || !wz.RunsOn(host.OS) {
		return fmt.Errorf("no walkthrough with ID %q runs on %s", opts.wizard, host.OS.Display())
	}

	session := wizard.Start(wz, newApplier(audit, host, opts), opts.timeout)
	reader := bufio.NewReader(stdin)

	fmt.Fprintf(w, "%s\n%s\n", bundle.T(wz.Title), bundle.T(wz.Complaint))

	progress, err := session.Next(ctx)
	if err != nil {
		return err
	}

	for progress.Outcome == wizard.OutcomeRunning {
		step := progress.Step
		fmt.Fprintf(w, "\n%s %s\n  %s\n",
			bundle.T(wizard.KeyStepChecking), bundle.T(step.Title), bundle.T(step.Finding.Summary, step.Finding.Args...))
		if step.Err != "" {
			fmt.Fprintf(w, "  %s\n", step.Err)
		}
		if progress.Advice != "" {
			fmt.Fprintf(w, "\n  %s\n", bundle.T(progress.Advice))
		}

		if progress.Offer == nil || !progress.Offer.Applicable() {
			if progress.Offer != nil && progress.Offer.Blocked != "" {
				fmt.Fprintf(w, "  %s\n", bundle.T("agent.fix.blocked", bundle.T(progress.Offer.Blocked)))
			}
			if progress, err = session.Skip(ctx); err != nil {
				return err
			}
			continue
		}

		fmt.Fprintln(w)
		writePlan(w, bundle, *progress.Offer)

		fmt.Fprintf(w, "\n%s ", bundle.T("agent.fix.confirm", progress.Offer.FixID))
		if !typed(reader, progress.Offer.FixID) {
			fmt.Fprintf(w, "%s\n", bundle.T("agent.fix.aborted"))
			if progress, err = session.Skip(ctx); err != nil {
				return err
			}
			continue
		}

		confirmation := remediate.Confirmation{
			Token:                     progress.Offer.Token,
			Acknowledged:              progress.Offer.Explanation.Changes,
			AcceptWithoutRestorePoint: !progress.Offer.Restore.Available,
		}
		if progress, err = session.Confirm(ctx, confirmation); err != nil {
			return err
		}
	}

	writeEscalation(w, bundle, session.Escalate())
	return nil
}

// writeEscalation prints the handover: what was asked, and what came of it.
func writeEscalation(w io.Writer, bundle *i18n.Bundle, escalation wizard.Escalation) {
	fmt.Fprintf(w, "\n%s\n", bundle.T(wizard.OutcomeKey(escalation.Outcome)))

	if len(escalation.Steps) == 0 {
		return
	}
	fmt.Fprintf(w, "\n%s\n", bundle.T("agent.wizard.summary"))
	for _, step := range escalation.Steps {
		fmt.Fprintf(w, "  [%s] %s — %s\n",
			step.Status, step.StepID, bundle.T(step.Finding.Summary, step.Finding.Args...))
	}
}

// typed reads one line and reports whether it is exactly want. Case and
// surrounding space are forgiven; a different word is not.
func typed(r *bufio.Reader, want string) bool {
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		// No input at all — a pipe, a service, a script. Silence is not
		// consent, so the change does not happen.
		return false
	}
	return strings.EqualFold(strings.TrimSpace(line), want)
}
