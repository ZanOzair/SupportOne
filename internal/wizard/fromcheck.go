package wizard

import (
	"context"
	"fmt"
	"time"

	"github.com/ZanOzair/SupportOne/internal/checks"
)

// FromCheck turns a registered diagnostic check into a wizard question.
//
// The checks already know how to read each platform honestly, including when
// they cannot read it at all. Building a wizard step on one means the wizard
// inherits that, rather than growing a second, shallower way to ask the same
// thing — and a check that reports Unknown produces a step that reports
// Unknown, never one that quietly passes.
func FromCheck(registry *checks.Registry, checkID string, timeout time.Duration) Probe {
	return func(ctx context.Context) (Finding, error) {
		check, ok := registry.Get(checkID)
		if !ok {
			return Finding{}, fmt.Errorf("wizard: no check with ID %q is compiled in", checkID)
		}

		res := checks.Run(ctx, check, timeout)
		return FindingFor(res), nil
	}
}

// FindingFor maps a check result onto a wizard finding.
func FindingFor(res checks.Result) Finding {
	f := Finding{Summary: res.Summary, Args: res.Args}

	switch res.Severity {
	case checks.SeverityOK:
		f.OK = true
	case checks.SeverityUnknown:
		// A check that could not answer has not cleared anything.
		f.Unknown = true
	case checks.SeverityAttention, checks.SeverityUrgent:
	}
	return f
}
