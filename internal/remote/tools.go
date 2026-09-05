package remote

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

// knownTool is one entry in the compiled-in whitelist.
//
// Nothing outside this file decides what may be launched. There is no
// configuration file, no environment variable and no API field that adds a
// tool: a build either knows a program or it does not.
type knownTool struct {
	id   string
	name string

	// commands are the places to look, in order of preference. A bare name is
	// looked up on PATH; a path is checked as written. ${VAR} is expanded from
	// the environment, and a variable that is unset means that candidate is
	// skipped rather than turning into a path with a hole in it.
	commands []string
}

// knownTools is every remote-help program SupportOne will look for.
//
// The list is short on purpose. These are widely used tools with published
// security documentation; a program nobody can look up is not one to point a
// worried person at. SupportOne does not install, update, configure or
// register any of them, and being on this list is not an endorsement of the
// company behind it — it means the program is common enough that finding it
// already installed is worth reporting.
var knownTools = map[platform.OS][]knownTool{
	platform.Windows: {
		{
			id:   "quickassist",
			name: "Quick Assist",
			commands: []string{
				"quickassist.exe",
				`${SystemRoot}\System32\quickassist.exe`,
			},
		},
		{
			id:   "rustdesk",
			name: "RustDesk",
			commands: []string{
				"rustdesk.exe",
				`${ProgramFiles}\RustDesk\rustdesk.exe`,
			},
		},
		{
			id:   "anydesk",
			name: "AnyDesk",
			commands: []string{
				"AnyDesk.exe",
				`${ProgramFiles}\AnyDesk\AnyDesk.exe`,
				`${ProgramFiles(x86)}\AnyDesk\AnyDesk.exe`,
			},
		},
		{
			id:   "teamviewer",
			name: "TeamViewer",
			commands: []string{
				"TeamViewer.exe",
				`${ProgramFiles}\TeamViewer\TeamViewer.exe`,
				`${ProgramFiles(x86)}\TeamViewer\TeamViewer.exe`,
			},
		},
	},

	platform.Darwin: {
		{
			id:   "screensharing",
			name: "Screen Sharing",
			commands: []string{
				"/System/Applications/Utilities/Screen Sharing.app/Contents/MacOS/Screen Sharing",
				"/System/Library/CoreServices/Applications/Screen Sharing.app/Contents/MacOS/Screen Sharing",
			},
		},
		{
			id:   "rustdesk",
			name: "RustDesk",
			commands: []string{
				"/Applications/RustDesk.app/Contents/MacOS/rustdesk",
				"rustdesk",
			},
		},
		{
			id:   "anydesk",
			name: "AnyDesk",
			commands: []string{
				"/Applications/AnyDesk.app/Contents/MacOS/AnyDesk",
			},
		},
		{
			id:   "teamviewer",
			name: "TeamViewer",
			commands: []string{
				"/Applications/TeamViewer.app/Contents/MacOS/TeamViewer",
			},
		},
	},

	platform.Linux: {
		{
			id:       "rustdesk",
			name:     "RustDesk",
			commands: []string{"rustdesk"},
		},
		{
			id:       "anydesk",
			name:     "AnyDesk",
			commands: []string{"anydesk"},
		},
		{
			id:       "teamviewer",
			name:     "TeamViewer",
			commands: []string{"teamviewer"},
		},
	},
}

// errNoSuchTool is what look returns for a candidate that is not there. It is
// unexported because callers only ever ask whether a tool was found.
var errNoSuchTool = errors.New("remote: not found")

// look resolves one whitelist candidate to a path on this machine.
//
// A bare name goes through PATH. Anything else is checked where it is written,
// because the programs people actually have are installed into fixed places
// and are usually not on PATH at all.
func look(candidate string) (string, error) {
	expanded, ok := expand(candidate)
	if !ok {
		return "", errNoSuchTool
	}

	if !filepath.IsAbs(expanded) {
		return exec.LookPath(expanded)
	}

	info, err := os.Stat(expanded)
	if err != nil {
		return "", errNoSuchTool
	}
	if info.IsDir() {
		return "", errNoSuchTool
	}
	return expanded, nil
}

// expand fills in ${VAR} references. It reports false if any of them is unset,
// so that a missing ProgramFiles produces "not found" rather than a path that
// happens to resolve somewhere else.
func expand(candidate string) (string, bool) {
	complete := true
	out := os.Expand(candidate, func(key string) string {
		value := os.Getenv(key)
		if value == "" {
			complete = false
		}
		return value
	})
	return out, complete
}

// launch starts a whitelisted program and lets go of it.
//
// path is never assembled from anything a user typed: it is what look returned
// for a compiled-in candidate, so there is nothing to inject and no shell in
// the way. No arguments are passed either — the tool shows its own window and
// asks for its own code, which is where the technician's side of the exchange
// belongs.
//
// SupportOne does not wait for the program and does not supervise it. Once it
// is running it is the user's, and closing it is theirs to do.
func launch(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("remote: refusing to start %q: only a resolved path is launched", path)
	}

	// exec.Command, not exec.CommandContext: binding the program's life to
	// this request's context would kill the remote session the moment the API
	// call that started it returned.
	//
	// #nosec G204 -- path is what look() resolved for an entry in knownTools;
	// no part of it comes from user input, and no arguments are passed.
	cmd := exec.Command(path)

	// A remote-help tool is a windowed program, so this changes nothing for
	// the ones in knownTools. It is here because the agent has no console to
	// give away: if one of these ever ships a console launcher, it would open
	// a black window in the user's face at exactly the wrong moment.
	platform.NoConsoleWindow(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("remote: start %s: %w", filepath.Base(path), err)
	}

	// Reap it when it eventually exits, so an ended session does not leave a
	// zombie behind. This waits on the process, it does not control it.
	go func() { _ = cmd.Wait() }()
	return nil
}
