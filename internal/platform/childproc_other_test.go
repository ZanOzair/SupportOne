//go:build !windows

package platform

import (
	"os/exec"
	"testing"
)

func TestNoConsoleWindowIsHarmlessHere(t *testing.T) {
	cmd := exec.Command("echo", "ok")
	NoConsoleWindow(cmd)
	HideConsoleTool(cmd)

	if cmd.SysProcAttr != nil {
		t.Error("NoConsoleWindow touched SysProcAttr on a platform that has no console windows")
	}
	if err := cmd.Run(); err != nil {
		t.Errorf("the command no longer runs: %v", err)
	}
}
