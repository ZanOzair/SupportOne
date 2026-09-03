package platform

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunReadReturnsOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this uses a POSIX shell utility; the Windows path is covered by the check tests")
	}

	out, err := RunRead(context.Background(), "echo", "hello")
	if err != nil {
		t.Fatalf("RunRead: %v", err)
	}
	if strings.TrimSpace(string(out)) != "hello" {
		t.Errorf("output = %q, want hello", out)
	}
}

func TestRunReadReportsAMissingTool(t *testing.T) {
	_, err := RunRead(context.Background(), "supportone-no-such-tool")
	if !errors.Is(err, ErrToolMissing) {
		t.Errorf("err = %v, want ErrToolMissing so the check can name what is not installed", err)
	}
}

func TestRunReadRefusesShellMetacharacters(t *testing.T) {
	// Command names are compiled-in constants. Anything else is a bug worth
	// failing loudly rather than passing to the OS.
	for _, name := range []string{"echo hello; rm -rf /", "echo|cat", "$(whoami)", "echo`id`"} {
		if _, err := RunRead(context.Background(), name); err == nil {
			t.Errorf("RunRead(%q) succeeded, want a refusal", name)
		} else if errors.Is(err, ErrToolMissing) {
			t.Errorf("RunRead(%q) reported a missing tool; it should refuse the name outright", name)
		}
	}
}

func TestRunReadRespectsACancelledContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this uses a POSIX shell utility")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err := RunRead(ctx, "sleep", "10"); err == nil {
		t.Fatal("RunRead returned no error for a command that outlived its context")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("RunRead took %s to honour a 20ms deadline", elapsed)
	}
}

func TestRunReadCarriesTheReasonACommandFailed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this uses a POSIX shell utility")
	}

	_, err := RunRead(context.Background(), "ls", "/supportone-no-such-path")
	if err == nil {
		t.Fatal("RunRead succeeded for a path that does not exist")
	}
	if !strings.Contains(err.Error(), "ls") {
		t.Errorf("err = %v, want the command named", err)
	}
}

func TestOpenBrowserRefusesAnythingButTheAgentsOwnAddress(t *testing.T) {
	for _, target := range []string{
		"https://example.invalid",
		"file:///etc/passwd",
		"http://192.168.1.1/",
		"http://127.0.0.1.evil.example/",
		"",
	} {
		if err := OpenBrowser(context.Background(), target); err == nil {
			t.Errorf("OpenBrowser(%q) succeeded, want a refusal", target)
		}
	}
}
