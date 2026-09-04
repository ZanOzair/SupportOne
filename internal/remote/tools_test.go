package remote

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

func TestEveryTargetedOSHasAWhitelist(t *testing.T) {
	for _, os := range platform.All() {
		tools, ok := knownTools[os]
		if !ok || len(tools) == 0 {
			t.Errorf("%s has no remote-help tools listed", os)
		}
	}
}

func TestTheWhitelistIsWellFormed(t *testing.T) {
	for os, tools := range knownTools {
		seen := make(map[string]bool, len(tools))
		for _, tool := range tools {
			switch {
			case tool.id == "":
				t.Errorf("%s: a tool has no ID", os)
			case tool.name == "":
				t.Errorf("%s/%s: no name to show the user", os, tool.id)
			case len(tool.commands) == 0:
				t.Errorf("%s/%s: nowhere to look for it", os, tool.id)
			case seen[tool.id]:
				t.Errorf("%s: %q is listed twice", os, tool.id)
			case tool.id != strings.ToLower(tool.id):
				t.Errorf("%s: ID %q is not lowercase", os, tool.id)
			case strings.ContainsAny(tool.id, " \t/\\"):
				t.Errorf("%s: ID %q has whitespace or a separator in it", os, tool.id)
			}
			seen[tool.id] = true
		}
	}
}

func TestNoWhitelistEntryCarriesArguments(t *testing.T) {
	// A candidate is a program to find, never a command line. If one ever
	// grows a flag, the launcher would have to split it — and splitting a
	// string into a command is how injection starts.
	for os, tools := range knownTools {
		for _, tool := range tools {
			for _, candidate := range tool.commands {
				for _, flag := range []string{" -", " --", "&&", "|", ";", "$(", "`"} {
					if strings.Contains(candidate, flag) {
						t.Errorf("%s/%s: candidate %q contains %q", os, tool.id, candidate, flag)
					}
				}
			}
		}
	}
}

func TestExpandFillsInTheEnvironment(t *testing.T) {
	t.Setenv("SUPPORTONE_TEST_ROOT", filepath.FromSlash("/opt/root"))

	got, ok := expand("${SUPPORTONE_TEST_ROOT}/tool")
	if !ok {
		t.Fatal("expand reported an incomplete path for a variable that is set")
	}
	if want := filepath.FromSlash("/opt/root") + "/tool"; got != want {
		t.Errorf("expand = %q, want %q", got, want)
	}
}

func TestAnUnsetVariableMeansNotFoundRatherThanAPathWithAHoleInIt(t *testing.T) {
	t.Setenv("SUPPORTONE_TEST_MISSING", "")

	if got, ok := expand(`${SUPPORTONE_TEST_MISSING}\AnyDesk\AnyDesk.exe`); ok {
		t.Errorf("expand = %q, ok = true; an unset variable should not produce a usable path", got)
	}

	if _, err := look(`${SUPPORTONE_TEST_MISSING}\AnyDesk\AnyDesk.exe`); !errors.Is(err, errNoSuchTool) {
		t.Errorf("look error = %v, want errNoSuchTool", err)
	}
}

func TestExpandLeavesAPlainPathAlone(t *testing.T) {
	const path = "/Applications/RustDesk.app/Contents/MacOS/rustdesk"

	got, ok := expand(path)
	if !ok || got != path {
		t.Errorf("expand(%q) = %q, %v; want it unchanged", path, got, ok)
	}
}

func TestLookFindsAnAbsolutePathThatExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pretend-remote-tool")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o700); err != nil { // #nosec G306 -- a fake program this test then looks for
		t.Fatalf("write fixture: %v", err)
	}

	got, err := look(path)
	if err != nil {
		t.Fatalf("look: %v", err)
	}
	if got != path {
		t.Errorf("look = %q, want %q", got, path)
	}
}

func TestLookDoesNotMistakeADirectoryForAProgram(t *testing.T) {
	dir := t.TempDir()
	app := filepath.Join(dir, "RustDesk.app")
	if err := os.Mkdir(app, 0o700); err != nil {
		t.Fatalf("make fixture directory: %v", err)
	}

	if _, err := look(app); !errors.Is(err, errNoSuchTool) {
		t.Errorf("look at a directory = %v, want errNoSuchTool", err)
	}
}

func TestLookReportsAMissingPathAsMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nothing-is-here")

	if _, err := look(path); !errors.Is(err, errNoSuchTool) {
		t.Errorf("look = %v, want errNoSuchTool", err)
	}
}

func TestLookFallsBackToPathForABareName(t *testing.T) {
	dir := t.TempDir()
	name := "supportone-test-tool"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o700); err != nil { // #nosec G306 -- a fake program this test then looks for
		t.Fatalf("write fixture: %v", err)
	}
	t.Setenv("PATH", dir)

	got, err := look(name)
	if err != nil {
		t.Fatalf("look on PATH: %v", err)
	}
	if filepath.Base(got) != name {
		t.Errorf("look = %q, want something named %q", got, name)
	}

	if _, err := look("supportone-test-tool-that-is-not-there"); err == nil {
		t.Error("look found a program that does not exist")
	}
}

func TestLaunchRefusesAnythingThatIsNotAResolvedPath(t *testing.T) {
	err := launch(context.Background(), "rustdesk")
	if err == nil {
		t.Fatal("launch accepted a bare name")
	}
	if !strings.Contains(err.Error(), "resolved path") {
		t.Errorf("launch error = %v, want it to say why", err)
	}
}

func TestLaunchStopsIfTheContextIsAlreadyDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := launch(ctx, "/bin/true"); !errors.Is(err, context.Canceled) {
		t.Fatalf("launch error = %v, want context.Canceled", err)
	}
}

func TestLaunchReportsAProgramThatIsNotThere(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-program")

	err := launch(context.Background(), path)
	if err == nil {
		t.Fatal("launch reported success for a program that does not exist")
	}
	if !strings.Contains(err.Error(), "not-a-program") {
		t.Errorf("launch error = %v, want it to name the program", err)
	}
}

func TestLaunchStartsTheProgramAndDoesNotWaitForIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture is a shell script")
	}

	dir := t.TempDir()
	marker := filepath.Join(dir, "it-ran")
	script := filepath.Join(dir, "pretend-remote-tool")
	body := "#!/bin/sh\nsleep 0.2\ntouch " + marker + "\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil { // #nosec G306 -- a fake program this test then launches
		t.Fatalf("write fixture: %v", err)
	}

	if err := launch(context.Background(), script); err != nil {
		t.Fatalf("launch: %v", err)
	}

	// launch returned before the program finished, which is the point: a
	// remote session outlives the request that agreed to it.
	if _, err := os.Stat(marker); err == nil {
		t.Error("launch waited for the program to finish")
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the program never ran")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
