package events

import (
	"context"
	"strconv"
	"time"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

const journalctlExe = "journalctl"

// lookback is how far back the check reads. Seven days is long enough to show
// a repeating fault and short enough to stay fast.
const lookback = 7 * 24 * time.Hour

func collectEvents(ctx context.Context, run platform.Runner) ([]logEvent, time.Duration, error) {
	out, err := run(ctx, journalctlExe,
		"--priority", "err",
		"--since", "-7days",
		"--lines", strconv.Itoa(maxEvents),
		"--output", "json",
		"--no-pager",
	)
	if err != nil {
		return nil, lookback, err
	}
	return parseJournal(out), lookback, nil
}
