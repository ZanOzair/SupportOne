package platform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ErrToolMissing is returned when the tool a check needs is not installed.
// Checks translate it into an "unknown" result naming the missing tool, so the
// user learns why an answer is missing rather than seeing a silent gap.
var ErrToolMissing = errors.New("platform: required tool is not installed")

// maxOutput caps how much of a command's output is kept. OS tools are not
// hostile, but a runaway one should not exhaust memory during a snapshot.
const maxOutput = 8 << 20 // 8 MiB

// RunRead executes a read-only OS command and returns its standard output.
//
// Two rules make this safe to call from checks: the command name and its
// arguments are compiled-in constants, never assembled from user input or model
// output; and no shell is involved, so there is nothing to quote or escape.
// Arguments that vary (a device path, a day count) are values the agent itself
// derived, passed as separate argv entries.
func RunRead(ctx context.Context, name string, args ...string) ([]byte, error) {
	return run(ctx, name, args...)
}

// RunAction executes a compiled-in OS command that changes the machine.
//
// It is the same mechanism as RunRead — no shell, constant command names,
// argv passed as separate entries — under a different name, because the
// difference that matters is who may call it. Only a Fix calls RunAction, and
// only after the user has confirmed that specific change.
func RunAction(ctx context.Context, name string, args ...string) ([]byte, error) {
	return run(ctx, name, args...)
}

func run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if strings.ContainsAny(name, "|&;<>()$`\\\"'") {
		return nil, fmt.Errorf("platform: refusing to run %q: command names are compiled-in constants", name)
	}
	if _, err := exec.LookPath(name); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrToolMissing, name)
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204 -- name and args are compiled-in constants; see doc comment.
	NoConsoleWindow(cmd)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	out := stdout.Bytes()
	if len(out) > maxOutput {
		out = out[:maxOutput]
	}
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return out, fmt.Errorf("platform: %s: %w", name, ctxErr)
		}
		return out, fmt.Errorf("platform: %s: %w: %s", name, err, firstLine(stderr.String()))
	}
	return out, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// Runner is the seam checks use so their collectors can be tested against
// recorded tool output instead of a live machine.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)
