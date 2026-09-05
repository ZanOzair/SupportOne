package platform

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRunNativeWindowRefusesAddressesItDidNotMint(t *testing.T) {
	// The address is checked before anything else, so this holds whether or
	// not a window could have been opened on this machine.
	for _, target := range []string{"http://example.com/", "https://127.0.0.1:8080/", ""} {
		err := RunNativeWindow(NativeWindowOptions{URL: target})
		if err == nil {
			t.Fatalf("RunNativeWindow(%q) = nil, want a refusal", target)
		}
		if !strings.Contains(err.Error(), "loopback") {
			t.Errorf("RunNativeWindow(%q) error = %v, want the address refused first", target, err)
		}
	}
}

// TestNativeWindowNeedsTheLoaderOnDisk is the test that matters most in this
// file. The WebView2 library will, if the loader is not found as a file, map an
// embedded copy of it into the process without the operating system's loader.
// This project does not use that technique, and the way it is kept unreachable
// is by refusing the native window when the file is absent — as it is beside a
// test binary in a temporary directory.
func TestNativeWindowNeedsTheLoaderOnDisk(t *testing.T) {
	if NativeWindowAvailable() {
		t.Fatal("NativeWindowAvailable() = true with no loader beside the test binary")
	}

	err := RunNativeWindow(NativeWindowOptions{URL: "http://127.0.0.1:49152/"})
	if err == nil {
		t.Fatal("RunNativeWindow() = nil with no loader on disk")
	}
	if !strings.Contains(err.Error(), loaderDLL) {
		t.Errorf("RunNativeWindow() error = %v, want it to name the missing %s", err, loaderDLL)
	}
}

func TestLoaderIsLookedForBesideTheProgramNotTheWorkingDirectory(t *testing.T) {
	got, err := loaderPath()
	if err != nil {
		t.Fatalf("loaderPath() error = %v", err)
	}
	if filepath.Base(got) != loaderDLL {
		t.Errorf("loaderPath() = %q, want it to end in %s", got, loaderDLL)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("loaderPath() = %q, want an absolute path: a relative one would be answered by the working directory", got)
	}
}
