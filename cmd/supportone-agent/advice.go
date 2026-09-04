package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/ZanOzair/SupportOne/internal/assist"
	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/consent"
	"github.com/ZanOzair/SupportOne/internal/explain"
	"github.com/ZanOzair/SupportOne/internal/i18n"
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/redact"
)

// writeAdvice prints the offline explanation beneath one verdict.
//
// It is indented under the finding rather than shown as a separate section,
// because the answer to "what does that mean" belongs next to the thing it
// explains, not in a list the reader has to cross-reference.
func writeAdvice(w io.Writer, bundle *i18n.Bundle, explainer *explain.Explainer, res checks.Result) {
	advice, ok := explainer.For(res)
	if !ok {
		return
	}

	fmt.Fprintf(w, "      %s\n", bundle.T(advice.Cause))
	for _, step := range advice.Steps {
		fmt.Fprintf(w, "      - %s\n", bundle.T(step))
	}
	for _, id := range advice.Fixes {
		fmt.Fprintf(w, "      - %s\n", bundle.T("agent.advice.fix_available", id))
	}
	for _, id := range advice.Wizards {
		fmt.Fprintf(w, "      - %s\n", bundle.T("agent.advice.wizard_available", id))
	}
	fmt.Fprintln(w)
}

// askAssistant runs the Tier-2 flow from the terminal: show the exact bytes,
// ask, and send only if the answer is yes.
//
// Everything above this point in a run happened on this computer. This is the
// only thing SupportOne does that does not, and it is the only thing that asks
// a second time.
func askAssistant(
	ctx context.Context,
	stdin io.Reader,
	w io.Writer,
	bundle *i18n.Bundle,
	audit *consent.Log,
	host platform.Host,
	snap checks.Snapshot,
	opts options,
) error {
	assistant := newAssistant(audit, host, opts)

	// The terminal path always redacts. Someone piping a report into a model
	// endpoint is not sitting there weighing each field, and the protective
	// choice is the one to make on their behalf when they are not asked.
	payload, err := assistant.Prepare(snap, redact.Everything(), redact.CurrentIdentity())
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "\n%s\n", bundle.T("agent.assist.heading"))
	fmt.Fprintf(w, "%s\n", bundle.T("agent.assist.destination", payload.Host, payload.Model))
	fmt.Fprintf(w, "%s\n", bundle.T("agent.assist.size", payload.Bytes))
	fmt.Fprintf(w, "%s\n\n", bundle.T("agent.assist.redacted"))

	// The payload itself, in full. A summary of what would be sent is not
	// what the user agreed to; the bytes are.
	fmt.Fprintf(w, "%s\n%s\n\n", bundle.T("agent.assist.payload"), payload.Body)

	reader := bufio.NewReader(stdin)
	fmt.Fprintf(w, "%s ", bundle.T("agent.assist.confirm"))
	if !typed(reader, "send") {
		assistant.Discard(payload.Token)
		fmt.Fprintf(w, "%s\n", bundle.T("agent.assist.not_sent"))
		return nil
	}

	answer, err := assistant.Ask(ctx, payload.Token)
	if err != nil {
		return err
	}
	writeAnswer(w, bundle, answer)
	return nil
}

// writeAnswer prints what came back, marked as the model's words throughout.
func writeAnswer(w io.Writer, bundle *i18n.Bundle, answer assist.Answer) {
	model := answer.Model
	if model == "" {
		model = bundle.T("agent.assist.unnamed_model")
	}

	fmt.Fprintf(w, "\n%s\n", bundle.T("agent.assist.answer_from", model))
	if notes := strings.TrimSpace(answer.Notes); notes != "" {
		fmt.Fprintf(w, "\n%s\n", notes)
	}

	if len(answer.Fixes) > 0 {
		fmt.Fprintf(w, "\n%s\n", bundle.T("agent.assist.suggested"))
		for _, id := range answer.Fixes {
			fmt.Fprintf(w, "  - %s\n", bundle.T("agent.advice.fix_available", id))
		}
	}
	if answer.Discarded > 0 {
		// A model that keeps naming repairs that do not exist is worth
		// knowing about, so the count is shown rather than swallowed.
		fmt.Fprintf(w, "\n%s\n", bundle.T("agent.assist.discarded", answer.Discarded))
	}
	fmt.Fprintf(w, "\n%s\n", bundle.T("agent.assist.caveat"))
}
