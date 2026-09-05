package platform

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// Windows sorts programs into two kinds at link time. A console program always
// gets a black terminal window, whether it needs one or not; a windows (GUI)
// program never gets one, and writing to standard output goes nowhere.
//
// The agent wants both. Double-clicked from the Start Menu it should behave
// like an application — open a browser, show no terminal. Run from a prompt
// with --text or --version, it must print where the person can read it.
//
// The way out is to link it as a GUI program and, at startup, ask to borrow
// the console of whatever launched it. There is one when a terminal started
// it, and there is not when Explorer did. That single call is what tells the
// two cases apart.

var (
	kernel32      = windows.NewLazySystemDLL("kernel32.dll")
	attachConsole = kernel32.NewProc("AttachConsole")
)

// attachParentProcess is the pseudo-identifier meaning "whatever launched me".
const attachParentProcess = ^uint32(0) // (DWORD)-1

// AttachConsole borrows the console of the process that started this one, and
// reports whether there was one to borrow.
//
// False is the ordinary answer for a program somebody double-clicked, and it
// is not an error: it means output has no terminal to go to, so the caller
// should say what it needs to say some other way.
func AttachConsole() bool {
	ret, _, _ := attachConsole.Call(uintptr(attachParentProcess))
	if ret == 0 {
		return false
	}

	// The console is attached, but this process's standard handles still point
	// at the nothing a GUI program was given. Reopening them against the
	// console's own devices is what makes printing work.
	stdout := reopen("CONOUT$", windows.GENERIC_WRITE, windows.STD_OUTPUT_HANDLE)
	if stdout != nil {
		os.Stdout = stdout
	}
	if stderr := reopen("CONOUT$", windows.GENERIC_WRITE, windows.STD_ERROR_HANDLE); stderr != nil {
		os.Stderr = stderr
	}
	if stdin := reopen("CONIN$", windows.GENERIC_READ, windows.STD_INPUT_HANDLE); stdin != nil {
		os.Stdin = stdin
	}

	// Without a console there is nothing to print to and nothing was changed,
	// so the caller is told the truth either way.
	return stdout != nil
}

// reopen points one standard stream at the attached console.
func reopen(device string, access uint32, which uint32) *os.File {
	name, err := windows.UTF16PtrFromString(device)
	if err != nil {
		return nil
	}

	handle, err := windows.CreateFile(
		name, access,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_EXISTING, 0, 0,
	)
	if err != nil {
		return nil
	}

	// Set the process-wide handle too, so anything that asks Windows directly
	// rather than going through os.Stdout lands in the same place.
	_ = windows.SetStdHandle(which, handle)
	return os.NewFile(uintptr(handle), device)
}

// ShowMessage puts a message in front of somebody who has no terminal to read.
//
// It is used for one thing: telling a person the address to open when the
// agent could not open their browser for them. Without it, a double-clicked
// agent that failed to launch a browser would appear to do nothing at all.
func ShowMessage(title, body string) error {
	caption, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return fmt.Errorf("platform: message title: %w", err)
	}
	text, err := windows.UTF16PtrFromString(body)
	if err != nil {
		return fmt.Errorf("platform: message body: %w", err)
	}

	// MB_OK | MB_ICONINFORMATION | MB_SETFOREGROUND
	const style = 0x00000000 | 0x00000040 | 0x00010000
	if _, err := windows.MessageBox(0, text, caption, style); err != nil {
		return fmt.Errorf("platform: show message: %w", err)
	}
	return nil
}
