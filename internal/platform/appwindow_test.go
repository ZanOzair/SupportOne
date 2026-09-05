package platform

import (
	"errors"
	"strings"
	"testing"
)

func TestOpenAppWindowRefusesAddressesItDidNotMint(t *testing.T) {
	for _, target := range []string{
		"http://example.com/",
		"https://127.0.0.1:8080/",
		"file:///etc/passwd",
		"http://127.0.0.1.evil.example/",
		"",
	} {
		err := OpenAppWindow(target)
		if err == nil {
			t.Fatalf("OpenAppWindow(%q) = nil, want a refusal", target)
		}
		if errors.Is(err, ErrNoAppWindow) {
			t.Fatalf("OpenAppWindow(%q) reported a missing browser; the address should be refused first", target)
		}
		if !strings.Contains(err.Error(), "loopback") {
			t.Errorf("OpenAppWindow(%q) error = %v, want it to say why", target, err)
		}
	}
}

func TestAppWindowArgsAskForAWindowWithNoBrowserAroundIt(t *testing.T) {
	const target = "http://127.0.0.1:49152/?t=abc"
	args := appWindowArgs(target)

	if args[0] != "--app="+target {
		t.Fatalf("first argument = %q, want the --app flag carrying the address", args[0])
	}
	for _, want := range []string{"--no-first-run", "--no-default-browser-check"} {
		if !containsArg(args, want) {
			t.Errorf("args %q missing %s", args, want)
		}
	}
	// The address travels inside one argument. Nothing splits it, and no
	// shell sees it.
	for _, arg := range args[1:] {
		if strings.Contains(arg, target) {
			t.Errorf("address repeated in %q; it belongs in --app alone", arg)
		}
	}
}

func TestAppWindowCandidatesAreEitherPathsOrBareNames(t *testing.T) {
	for _, candidate := range appWindowCandidates() {
		if candidate == "" {
			t.Fatal("empty candidate: findAppWindowProgram would look up nothing")
		}
		// findAppWindowProgram tells the two forms apart by looking for a
		// separator, so a name must not contain one and a path must.
		hasSeparator := strings.ContainsAny(candidate, `/\`)
		switch Current() {
		case Linux:
			if hasSeparator {
				t.Errorf("candidate %q: Linux entries are command names looked up on PATH", candidate)
			}
		case Windows, Darwin:
			if !hasSeparator {
				t.Errorf("candidate %q: neither Windows nor macOS puts browsers on PATH", candidate)
			}
		}
	}
}

func TestExpandPathSkipsCandidatesWhoseVariablesAreUnset(t *testing.T) {
	t.Setenv("SUPPORTONE_TEST_ROOT", `C:\Program Files`)

	got, ok := expandPath(`${SUPPORTONE_TEST_ROOT}\Browser\browser.exe`)
	if !ok {
		t.Fatal("expandPath refused a path whose variable is set")
	}
	if want := `C:\Program Files\Browser\browser.exe`; got != want {
		t.Errorf("expandPath = %q, want %q", got, want)
	}

	// An unset variable must not collapse into a path rooted at the drive.
	// That would name a real, different location.
	if _, ok := expandPath(`${SUPPORTONE_TEST_UNSET}\Browser\browser.exe`); ok {
		t.Error("expandPath accepted a path whose variable is unset")
	}
	if _, ok := expandPath("${unterminated"); ok {
		t.Error("expandPath accepted an unterminated variable")
	}
}

func TestExpandPathLeavesOrdinaryPathsAlone(t *testing.T) {
	const path = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	got, ok := expandPath(path)
	if !ok || got != path {
		t.Errorf("expandPath(%q) = %q, %v; want it unchanged", path, got, ok)
	}
}

// FuzzExpandPath checks the small parser in expandPath: it must not panic, and
// whatever it accepts must be free of the syntax it is supposed to have
// consumed.
func FuzzExpandPath(f *testing.F) {
	for _, seed := range []string{
		"",
		"$",
		"${",
		"${}",
		"${A}",
		"${A}${B}",
		`${ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe`,
		"/usr/bin/chromium",
		"$}{",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		got, ok := expandPath(raw)
		if !ok {
			return
		}
		if strings.Contains(got, "${") {
			t.Errorf("expandPath(%q) accepted %q with a variable still in it", raw, got)
		}
	})
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
