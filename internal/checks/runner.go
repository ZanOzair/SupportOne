package checks

import (
	"context"
	"fmt"
	"time"
)

// DefaultTimeout bounds a single check. A check that cannot answer in this
// long reports Unknown rather than hanging the snapshot.
const DefaultTimeout = 30 * time.Second

// Run executes c under a timeout and always returns a Result.
//
// A check that fails, times out or panics yields SeverityUnknown with the
// reason recorded — never a fabricated OK, and never a silently dropped check.
func Run(ctx context.Context, c Check, timeout time.Duration) Result {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	started := time.Now()
	res, err := runGuarded(ctx, c)
	if err != nil {
		return Result{
			CheckID:   c.ID(),
			Severity:  SeverityUnknown,
			Err:       err.Error(),
			StartedAt: started,
			Duration:  time.Since(started),
		}
	}

	res.CheckID = c.ID()
	res.StartedAt = started
	res.Duration = time.Since(started)
	if !res.Severity.Valid() {
		res.Err = fmt.Sprintf("check returned invalid severity %q", res.Severity)
		res.Severity = SeverityUnknown
	}
	return res
}

// runGuarded turns a panicking check into an error so one bad module cannot
// take down a snapshot the user is waiting on.
func runGuarded(ctx context.Context, c Check) (res Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("check %s panicked: %v", c.ID(), r)
		}
	}()
	return c.Run(ctx)
}
