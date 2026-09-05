package platform

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNoAppWindow is returned when nothing installed on this computer can show
// a window without browser furniture around it. It is the caller's cue to fall
// back to the ordinary browser rather than to give up.
var ErrNoAppWindow = errors.New("platform: no installed browser here can open a window of its own")

// OpenAppWindow shows the agent's interface in a window of its own: no address
// bar, no tabs, no bookmarks, its own icon in the taskbar or dock.
//
// It does this by asking a Chromium-family browser the user already has for an
// "app window" — the same window Chrome and Edge use for installed web apps.
// Nothing is downloaded, no extra runtime is required and no window toolkit is
// linked in, which is what keeps the agent one static file that cross-compiles
// from a single machine.
//
// The program is found in a compiled-in list of the places these browsers
// install themselves, and the URL is the agent's own loopback address, checked
// before anything is launched. Neither comes from user input, and no shell is
// involved.
func OpenAppWindow(target string) error {
	if err := checkLoopback(target); err != nil {
		return err
	}

	program := findAppWindowProgram()
	if program == "" {
		return ErrNoAppWindow
	}

	// #nosec G204 -- program comes from the compiled-in table below, found on
	// disk rather than named by anyone, and target is the agent's own loopback
	// URL, checked above.
	cmd := exec.Command(program, appWindowArgs(target)...)
	NoConsoleWindow(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("platform: %s would not start: %w", filepath.Base(program), err)
	}

	// Deliberately not exec.CommandContext: the window belongs to the person
	// using it, so the agent shutting down must not close it. It is still
	// waited on, in the background, so the process is reaped rather than left
	// behind as a zombie for as long as the agent runs.
	go func() { _ = cmd.Wait() }()
	return nil
}

// appWindowArgs is what turns a browser into a plain window.
//
// --app is the whole trick: it asks for a window with no address bar, tab
// strip or navigation buttons. Hiding the address bar also keeps this session's
// token off the screen, which it is not today. The rest suppress the questions
// a browser asks the first time it is started this way, because they would
// appear in front of the interface and are not this program's to ask.
func appWindowArgs(target string) []string {
	return []string{
		"--app=" + target,
		"--window-size=1160,860",
		"--no-first-run",
		"--no-default-browser-check",
	}
}

// appWindowCandidates lists, in preference order, the programs that can open a
// window without browser furniture, and where to find them.
//
// An entry containing a path separator is a place these browsers install
// themselves and is looked for on disk; an entry without one is a command name
// and is looked up on PATH. Windows and macOS need the first form because
// neither puts its browsers on PATH.
func appWindowCandidates() []string {
	switch Current() {
	case Windows:
		// Edge is first because every supported version of Windows has it,
		// which makes this the case that decides whether the feature is real.
		return []string{
			`${ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe`,
			`${ProgramFiles}\Microsoft\Edge\Application\msedge.exe`,
			`${ProgramFiles}\Google\Chrome\Application\chrome.exe`,
			`${ProgramFiles(x86)}\Google\Chrome\Application\chrome.exe`,
			`${LOCALAPPDATA}\Google\Chrome\Application\chrome.exe`,
			`${ProgramFiles}\BraveSoftware\Brave-Browser\Application\brave.exe`,
			`${ProgramFiles(x86)}\BraveSoftware\Brave-Browser\Application\brave.exe`,
			`${ProgramFiles}\Vivaldi\Application\vivaldi.exe`,
		}
	case Darwin:
		// Safari has no equivalent, so a Mac with only Safari falls back to a
		// browser tab. That is a real gap and it is not worth linking a window
		// toolkit into every build to close it.
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"${HOME}/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"${HOME}/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Vivaldi.app/Contents/MacOS/Vivaldi",
		}
	case Linux:
		return []string{
			"google-chrome-stable",
			"google-chrome",
			"chromium",
			"chromium-browser",
			"microsoft-edge-stable",
			"microsoft-edge",
			"brave-browser",
			"vivaldi-stable",
		}
	default:
		return nil
	}
}

// findAppWindowProgram returns the first candidate that is actually installed,
// or an empty string when none is.
func findAppWindowProgram() string {
	for _, candidate := range appWindowCandidates() {
		path, ok := expandPath(candidate)
		if !ok {
			continue
		}

		if !strings.ContainsAny(path, `/\`) {
			if found, err := exec.LookPath(path); err == nil {
				return found
			}
			continue
		}

		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path
		}
	}
	return ""
}

// expandPath fills ${VAR} from the environment.
//
// It reports false when a variable the path needs is unset or empty, so the
// candidate is skipped rather than collapsing into a path that starts at the
// filesystem root — ${ProgramFiles}\Microsoft\... becoming \Microsoft\... would
// be a different question than the one being asked.
func expandPath(raw string) (string, bool) {
	var out strings.Builder
	for i := 0; i < len(raw); {
		if raw[i] != '$' || i+1 >= len(raw) || raw[i+1] != '{' {
			out.WriteByte(raw[i])
			i++
			continue
		}

		end := strings.IndexByte(raw[i+2:], '}')
		if end < 0 {
			return "", false
		}
		value := os.Getenv(raw[i+2 : i+2+end])
		if value == "" {
			return "", false
		}
		out.WriteString(value)
		i += 2 + end + 1
	}
	return out.String(), true
}
