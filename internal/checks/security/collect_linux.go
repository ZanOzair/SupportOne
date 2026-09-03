package security

import (
	"context"

	"github.com/ZanOzair/supportone/internal/platform"
)

const lsblkExe = "lsblk"

func collectPosture(ctx context.Context, run platform.Runner) (postureFacts, error) {
	facts := postureFacts{
		DiskEncryption: stateUnknown,
		Firewall:       stateUnknown,
		// Desktop Linux has no equivalent of the registered-antivirus service
		// Windows exposes. Reporting "off" would imply something is missing
		// that was never there.
		Antivirus: stateNotApplicable,
		Notes:     map[string]string{},
	}

	out, err := run(ctx, lsblkExe, "-J", "-o", "NAME,TYPE,FSTYPE")
	if err != nil {
		facts.Notes["disk_encryption"] = err.Error()
	} else if encryption, parseErr := parseLUKS(out); parseErr != nil {
		facts.Notes["disk_encryption"] = parseErr.Error()
	} else {
		facts.DiskEncryption = encryption
	}

	// Reading the packet filter needs root on every mainstream distribution,
	// and the agent does not ask for elevation just to look. Saying so is
	// more useful than reporting a firewall state we did not read.
	facts.Notes["firewall"] = "reading the firewall rules on Linux requires administrator rights"
	return facts, nil
}
