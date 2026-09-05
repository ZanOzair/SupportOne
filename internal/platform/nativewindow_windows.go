package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// loaderDLL is the Microsoft redistributable that starts the WebView2 runtime.
// It ships beside the executable; see nativeWindowReady for why that placement
// is load-bearing rather than incidental.
const loaderDLL = "WebView2Loader.dll"

// webView2Client is the Evergreen WebView2 Runtime's update client GUID. Its
// presence under EdgeUpdate is how Microsoft documents detecting the runtime,
// and reading it costs nothing — the alternative is asking the loader, which
// is exactly the call that must not be made before the checks below pass.
const webView2Client = `SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`

var loadLoaderOnce sync.Once

// NativeWindowAvailable reports whether this computer can show the interface in
// a window the agent owns, with no browser process involved.
//
// Two things must be true, and both are checked before the WebView2 library is
// touched at all. That ordering is the point of this function.
//
// The runtime must be installed. It is part of Windows 11 and reaches Windows
// 10 through Edge, so this is nearly always true, but "nearly always" is not
// something to find out by crashing.
//
// The loader must be on disk beside the executable. When it is missing, the
// library's own fallback maps an embedded copy of the DLL into the process
// without the operating system's loader — a reflective load. That is a
// technique this project will not use: it is what security tooling flags, and
// this program has to be defensible to the same tooling it reports on. So the
// loader is shipped as a file, and when it is absent the agent gives up on the
// native window rather than reaching for the fallback.
func NativeWindowAvailable() bool {
	return nativeWindowReady() == nil
}

func nativeWindowReady() error {
	loader, err := loaderPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(loader); err != nil {
		return fmt.Errorf("platform: %s is not beside the program: %w", loaderDLL, err)
	}
	if !webView2RuntimeInstalled() {
		return errors.New("platform: the WebView2 runtime is not installed")
	}
	return nil
}

// loaderPath is the loader's location beside the executable, resolved from the
// program's own path rather than the working directory: a DLL loaded by bare
// name can otherwise be answered by whatever happens to sit in the folder the
// user launched from.
func loaderPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("platform: cannot locate this program on disk: %w", err)
	}
	return filepath.Join(filepath.Dir(exe), loaderDLL), nil
}

func webView2RuntimeInstalled() bool {
	// Per-machine first, then per-user: both are real installations.
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		key, err := registry.OpenKey(root, webView2Client, registry.QUERY_VALUE|registry.WOW64_32KEY)
		if err != nil {
			continue
		}
		version, _, err := key.GetStringValue("pv")
		_ = key.Close()
		if err == nil && version != "" && version != "0.0.0.0" {
			return true
		}
	}
	return false
}

// NativeWindowOptions describes the window to open.
type NativeWindowOptions struct {
	Title  string
	URL    string
	Width  uint
	Height uint

	// DataPath is where the runtime keeps its own cache and storage for this
	// window. It belongs under the agent's own directory so that closing the
	// program leaves nothing in the user's browser profile.
	DataPath string
}

// RunNativeWindow opens the interface in a window the agent owns and blocks
// until the user closes it.
//
// There is no address bar, no tabs, no browser process and no browser profile:
// the window is this program's, and it appears in the taskbar and Alt-Tab as
// this program. Closing it returns, which is how the caller learns to stop.
func RunNativeWindow(opts NativeWindowOptions) error {
	if err := checkLoopback(opts.URL); err != nil {
		return err
	}
	if err := nativeWindowReady(); err != nil {
		return err
	}

	// Load the loader ourselves, by absolute path. The library would otherwise
	// find it by bare name, and this is both the belt for that search order and
	// the guarantee that its in-memory fallback is never reached: once the
	// module is loaded under this name, the library's lazy lookup resolves to
	// this one.
	var loadErr error
	loadLoaderOnce.Do(func() {
		loader, err := loaderPath()
		if err != nil {
			loadErr = err
			return
		}
		if _, err := windows.LoadLibraryEx(loader, 0, windows.LOAD_WITH_ALTERED_SEARCH_PATH); err != nil {
			loadErr = fmt.Errorf("platform: cannot load %s: %w", loaderDLL, err)
		}
	})
	if loadErr != nil {
		return loadErr
	}

	// A Win32 window must be created and pumped on one thread, and Go will
	// otherwise move this goroutine between threads whenever it likes.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	view := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		DataPath:  opts.DataPath,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  opts.Title,
			Width:  opts.Width,
			Height: opts.Height,
			// The icon already compiled into this executable, so the window,
			// the taskbar button and the Alt-Tab entry all show the same one.
			IconId: 1,
			Center: true,
		},
	})
	if view == nil {
		return errors.New("platform: the window could not be created")
	}
	defer view.Destroy()

	view.Navigate(opts.URL)
	view.Run()
	return nil
}
