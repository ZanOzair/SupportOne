package updates

import (
	"context"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

// Compiled-in query; see platform.RunRead. It reads two local records — the
// Windows Update agent's last successful install and the newest installed
// hotfix — and contacts nothing.
const (
	psExe = "powershell"

	queryUpdates = `$k = Get-ItemProperty -Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\Results\Install' -ErrorAction SilentlyContinue; ` +
		`$h = Get-CimInstance Win32_QuickFixEngineering -ErrorAction SilentlyContinue | ` +
		`Sort-Object InstalledOn -Descending | Select-Object -First 1; ` +
		`[pscustomobject]@{LastSuccess=$k.LastSuccessTime; LastHotFix=$h.InstalledOn} | ConvertTo-Json -Compress`
)

func collectUpdates(ctx context.Context, run platform.Runner) (updateFacts, error) {
	out, err := run(ctx, psExe, "-NoProfile", "-NonInteractive", "-Command", queryUpdates)
	if err != nil {
		return updateFacts{Pending: -1}, err
	}
	return parseWindowsUpdates(out)
}
