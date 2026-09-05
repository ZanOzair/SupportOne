//go:build !windows

package platform

import (
	"strings"
	"testing"
)

func TestNativeWindowIsNotAvailableHere(t *testing.T) {
	if NativeWindowAvailable() {
		t.Fatal("NativeWindowAvailable() = true; only Windows can draw its own window without cgo")
	}

	err := RunNativeWindow(NativeWindowOptions{URL: "http://127.0.0.1:49152/"})
	if err == nil {
		t.Fatal("RunNativeWindow() = nil; it cannot succeed on this operating system")
	}
	if !strings.Contains(err.Error(), "cgo") {
		t.Errorf("RunNativeWindow() error = %v, want it to say why this OS is excluded", err)
	}
}
