package platform

import (
	"os/exec"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func TestNoConsoleWindowAsksForNoConsole(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "echo")
	NoConsoleWindow(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("NoConsoleWindow left SysProcAttr nil")
	}
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Error("CREATE_NO_WINDOW not set: every check would flash a console window")
	}
	// HideWindow sets SW_HIDE, which applies to windowed programs too. Setting
	// it here would hide the browser the agent opens as a fallback, so it must
	// stay off.
	if cmd.SysProcAttr.HideWindow {
		t.Error("HideWindow set: that hides windowed programs, including the browser")
	}
}

func TestNoConsoleWindowKeepsFlagsAlreadySet(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "echo")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}

	NoConsoleWindow(cmd)

	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NEW_PROCESS_GROUP == 0 {
		t.Error("an existing creation flag was discarded")
	}
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Error("CREATE_NO_WINDOW not added alongside the existing flag")
	}
}

// TestReadRunsWithoutAConsoleWindow is the end-to-end version: the helper is
// only useful if the code path every check goes through actually applies it.
func TestReadRunsWithoutAConsoleWindow(t *testing.T) {
	out, err := RunRead(t.Context(), "cmd", "/c", "echo ok")
	if err != nil {
		t.Fatalf("RunRead: %v", err)
	}
	if len(out) == 0 {
		t.Error("RunRead returned nothing; suppressing the window must not suppress the output")
	}
}
