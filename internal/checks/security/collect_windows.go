package security

import (
	"context"

	"github.com/ZanOzair/supportone/internal/platform"
)

// Compiled-in queries; see platform.RunRead.
const (
	psExe = "powershell"

	queryFirewall = `Get-NetFirewallProfile | Select-Object Name,Enabled | ConvertTo-Json -Compress`

	queryAntivirus = `Get-CimInstance -Namespace root\SecurityCenter2 -ClassName AntiVirusProduct -ErrorAction SilentlyContinue | ` +
		`Select-Object displayName,productState | ConvertTo-Json -Compress`

	// BitLocker status needs administrator rights. SilentlyContinue makes an
	// unprivileged run report nothing, which the check records as unknown
	// rather than as an unencrypted disk.
	queryBitLocker = `Get-BitLockerVolume -MountPoint $env:SystemDrive -ErrorAction SilentlyContinue | ` +
		`Select-Object ProtectionStatus | ConvertTo-Json -Compress`
)

func psArgs(query string) []string {
	return []string{"-NoProfile", "-NonInteractive", "-Command", query}
}

func collectPosture(ctx context.Context, run platform.Runner) (postureFacts, error) {
	facts := postureFacts{
		DiskEncryption: stateUnknown,
		Firewall:       stateUnknown,
		Antivirus:      stateUnknown,
		Notes:          map[string]string{},
	}

	if out, err := run(ctx, psExe, psArgs(queryBitLocker)...); err != nil {
		facts.Notes["disk_encryption"] = err.Error()
	} else if encryption, parseErr := parseWindowsBitLocker(out); parseErr != nil {
		facts.Notes["disk_encryption"] = parseErr.Error()
	} else {
		facts.DiskEncryption = encryption
		if encryption == stateUnknown {
			facts.Notes["disk_encryption"] = "reading BitLocker status requires administrator rights"
		}
	}

	if out, err := run(ctx, psExe, psArgs(queryFirewall)...); err != nil {
		facts.Notes["firewall"] = err.Error()
	} else if firewall, parseErr := parseWindowsFirewall(out); parseErr != nil {
		facts.Notes["firewall"] = parseErr.Error()
	} else {
		facts.Firewall = firewall
	}

	if out, err := run(ctx, psExe, psArgs(queryAntivirus)...); err != nil {
		facts.Notes["antivirus"] = err.Error()
	} else if antivirus, name, parseErr := parseWindowsAntivirus(out); parseErr != nil {
		facts.Notes["antivirus"] = parseErr.Error()
	} else {
		facts.Antivirus = antivirus
		facts.AntivirusName = name
	}

	return facts, nil
}
