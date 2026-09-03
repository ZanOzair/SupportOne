package security

import (
	"context"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

const (
	fdesetupExe = "fdesetup"
	defaultsExe = "defaults"

	alfPrefs = "/Library/Preferences/com.apple.alf"
)

func collectPosture(ctx context.Context, run platform.Runner) (postureFacts, error) {
	facts := postureFacts{
		DiskEncryption: stateUnknown,
		Firewall:       stateUnknown,
		// macOS ships XProtect, which exposes no status to query. Reporting
		// a state we cannot read would be inventing one.
		Antivirus: stateNotApplicable,
		Notes:     map[string]string{},
	}

	if out, err := run(ctx, fdesetupExe, "status"); err != nil {
		facts.Notes["disk_encryption"] = err.Error()
	} else {
		facts.DiskEncryption = parseFDESetup(out)
	}

	if out, err := run(ctx, defaultsExe, "read", alfPrefs, "globalstate"); err != nil {
		facts.Notes["firewall"] = err.Error()
	} else {
		facts.Firewall = parseALFGlobalState(out)
	}
	return facts, nil
}
