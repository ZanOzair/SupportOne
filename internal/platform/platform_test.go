package platform

import (
	"runtime"
	"testing"
)

func TestCurrentMatchesRuntime(t *testing.T) {
	got := Current()
	switch runtime.GOOS {
	case "windows", "darwin", "linux":
		if string(got) != runtime.GOOS {
			t.Errorf("Current() = %q, want %q", got, runtime.GOOS)
		}
		if !got.Valid() {
			t.Errorf("Current() = %q is not Valid()", got)
		}
	default:
		if got != "" {
			t.Errorf("Current() = %q on unsupported %q, want empty", got, runtime.GOOS)
		}
	}
}

func TestValid(t *testing.T) {
	for _, os := range All() {
		if !os.Valid() {
			t.Errorf("%q.Valid() = false", os)
		}
	}
	for _, os := range []OS{"", "plan9", "Windows"} {
		if os.Valid() {
			t.Errorf("%q.Valid() = true, want false", os)
		}
	}
}

func TestDisplayNamesTheOSAsUsersKnowIt(t *testing.T) {
	if got := Darwin.Display(); got != "macOS" {
		t.Errorf("Darwin.Display() = %q, want macOS", got)
	}
}

func TestIsElevatedDoesNotError(t *testing.T) {
	// The value depends on how the test runs; that it answers at all is what
	// the agent relies on to decide whether to offer an elevated check.
	if _, err := IsElevated(); err != nil {
		t.Errorf("IsElevated: %v", err)
	}
}

func TestCurrentHostReportsArch(t *testing.T) {
	host := CurrentHost()
	if host.OS != Current() {
		t.Errorf("CurrentHost().OS = %q, want %q", host.OS, Current())
	}
	if host.Arch != runtime.GOARCH {
		t.Errorf("CurrentHost().Arch = %q, want %q", host.Arch, runtime.GOARCH)
	}
}

func TestDisplayCoversEveryTargetedOS(t *testing.T) {
	want := map[OS]string{Windows: "Windows", Darwin: "macOS", Linux: "Linux"}
	for _, os := range All() {
		if got := os.Display(); got != want[os] {
			t.Errorf("%q.Display() = %q, want %q", os, got, want[os])
		}
	}
	if got := OS("plan9").Display(); got != "plan9" {
		t.Errorf("unknown OS Display() = %q, want the raw value echoed back", got)
	}
}
