//go:build !windows

package platform

import "errors"

// NativeWindowOptions describes the window to open.
type NativeWindowOptions struct {
	Title    string
	URL      string
	Width    uint
	Height   uint
	DataPath string
}

// NativeWindowAvailable reports whether the agent can draw its own window.
//
// Only Windows can, and the reason is worth stating where someone will look for
// it. Drawing a window on macOS means WKWebView through the Objective-C
// runtime, and on Linux it means WebKitGTK; both need cgo, a platform SDK at
// build time, and on Linux a library the user must already have installed.
// That would cost the single static binary and the ability to build every
// target from one machine — for a window that looks the same as the one a
// Chromium-family browser gives for free. Windows is the exception because
// WebView2 is reachable without cgo and ships with the system.
func NativeWindowAvailable() bool { return false }

// RunNativeWindow always fails here; callers fall back to an app window.
func RunNativeWindow(_ NativeWindowOptions) error {
	return errors.New("platform: this operating system has no window the agent can draw without cgo")
}
