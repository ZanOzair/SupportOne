package checks

import (
	"errors"
	"fmt"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

// New builds a result carrying an i18n message key and the values that fill it.
//
// Summary is a key, not a sentence: the same result renders in every supported
// language from one code path.
func New(severity Severity, summary string, args ...any) Result {
	return Result{Severity: severity, Summary: summary, Args: args}
}

// OK reports that a check looked and found nothing wrong. Reporting zero
// problems is a valid, useful answer.
func OK(summary string, args ...any) Result { return New(SeverityOK, summary, args...) }

// Attention reports something worth acting on, but not today.
func Attention(summary string, args ...any) Result { return New(SeverityAttention, summary, args...) }

// Urgent reports something that risks data or downtime now.
func Urgent(summary string, args ...any) Result { return New(SeverityUrgent, summary, args...) }

// Unknown reports that the check could not determine an answer, and why. It is
// never a stand-in for OK.
func Unknown(summary string, args ...any) Result { return New(SeverityUnknown, summary, args...) }

// UnknownFor turns an error from a collector into an honest Unknown result: a
// missing tool is named as such, everything else carries the underlying reason.
func UnknownFor(err error) Result {
	if errors.Is(err, platform.ErrToolMissing) {
		res := Unknown(KeyToolMissing, toolName(err))
		res.Err = err.Error()
		return res
	}
	res := Unknown(KeyCheckFailed)
	res.Err = err.Error()
	return res
}

func toolName(err error) string {
	var missing string
	if _, scanErr := fmt.Sscanf(err.Error(), "platform: required tool is not installed: %s", &missing); scanErr == nil {
		return missing
	}
	return "a required tool"
}

// With attaches the structured evidence behind the summary. This is what the
// report shows and what the user reviews and redacts before sending anything.
func (r Result) With(detail map[string]any) Result {
	r.Detail = detail
	return r
}

// Message keys shared by every check.
const (
	KeyToolMissing = "check.unknown.tool_missing"
	KeyCheckFailed = "check.unknown.failed"
	KeyNeedsAdmin  = "check.unknown.needs_admin"
)

// PluralKey picks the singular variant of a message key when the count is one.
//
// English needs "1 drive checked" where it needs "3 drives checked"; Bahasa
// Melayu does not inflect, so its catalog carries the same text under both
// keys. Languages with richer plural rules can be given more variants without
// changing any check.
func PluralKey(base string, n int) string {
	if n == 1 {
		return base + ".one"
	}
	return base
}
