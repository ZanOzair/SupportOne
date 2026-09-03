package backup

import (
	"context"
	"fmt"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

// Compiled-in queries; see platform.RunRead for why this is not shell
// construction.
const (
	psExe = "powershell"

	// Volume Shadow Copy is what "Previous Versions" and System Protection
	// both rest on, and it is the one backup mechanism Windows ships that is
	// readable without configuring anything. File History and third-party
	// tools are not read here, which is why a negative result is worded as
	// "none SupportOne can see".
	queryShadowCopies = `$copy = Get-CimInstance Win32_ShadowCopy -ErrorAction SilentlyContinue | ` +
		`Sort-Object InstallDate -Descending | Select-Object -First 1; ` +
		`[pscustomobject]@{` +
		`Configured=($null -ne $copy); ` +
		`InstallDate=$copy.InstallDate; ` +
		`Volume=$copy.VolumeName` +
		`} | ConvertTo-Json -Compress`
)

func collectBackup(ctx context.Context, run platform.Runner) (backupFacts, error) {
	raw, err := run(ctx, psExe, "-NoProfile", "-NonInteractive", "-Command", queryShadowCopies)
	if err != nil {
		return backupFacts{}, fmt.Errorf("backup: read shadow copies: %w", err)
	}
	return parseShadowCopies(raw)
}
