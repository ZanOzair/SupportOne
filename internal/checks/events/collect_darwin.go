package events

import (
	"context"
	"time"

	"github.com/ZanOzair/supportone/internal/platform"
)

const logExe = "log"

// lookback is one day on macOS, not the seven the other platforms use.
// Querying the unified log is expensive, and a week-long query routinely takes
// longer than a snapshot should. The window is reported alongside the result so
// nobody reads a quiet day as a quiet week.
const lookback = 24 * time.Hour

func collectEvents(ctx context.Context, run platform.Runner) ([]logEvent, time.Duration, error) {
	out, err := run(ctx, logExe, "show",
		"--last", "24h",
		"--predicate", `messageType == "error" OR messageType == "fault"`,
		"--style", "ndjson",
	)
	if err != nil {
		return nil, lookback, err
	}

	events := parseMacLog(out)
	if len(events) > maxEvents {
		events = events[len(events)-maxEvents:]
	}
	return events, lookback, nil
}
