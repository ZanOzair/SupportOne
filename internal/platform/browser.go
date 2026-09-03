package platform

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// OpenBrowser asks the desktop to open a URL in the user's default browser.
//
// The URL is one the agent minted for its own loopback listener, and it is
// passed as a separate argument to a compiled-in command, so there is no shell
// and nothing to quote. A URL that did not come from the agent is refused
// outright.
func OpenBrowser(ctx context.Context, target string) error {
	if !strings.HasPrefix(target, "http://127.0.0.1:") {
		return fmt.Errorf("platform: refusing to open %q: only the agent's own loopback address is opened", target)
	}

	name, args := browserCommand(target)
	if name == "" {
		return fmt.Errorf("platform: no way to open a browser on %s", Current().Display())
	}
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("%w: %s", ErrToolMissing, name)
	}

	// #nosec G204 -- name comes from a compiled-in table and target is the
	// agent's own loopback URL, checked above.
	return exec.CommandContext(ctx, name, args...).Start()
}

// browserCommand returns the compiled-in way each desktop opens a URL.
func browserCommand(target string) (string, []string) {
	switch Current() {
	case Windows:
		// rundll32 hands the URL to whatever the user set as their browser,
		// without going through a shell that would interpret it.
		return "rundll32", []string{"url.dll,FileProtocolHandler", target}
	case Darwin:
		return "open", []string{target}
	case Linux:
		return "xdg-open", []string{target}
	default:
		return "", nil
	}
}
