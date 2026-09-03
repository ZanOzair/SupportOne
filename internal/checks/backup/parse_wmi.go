package backup

import (
	"fmt"
	"strings"

	"github.com/ZanOzair/SupportOne/internal/checks/cim"
)

// windowsBackup mirrors the object queryShadowCopies builds.
type windowsBackup struct {
	Configured  bool   `json:"Configured"`
	InstallDate string `json:"InstallDate"`
	Volume      string `json:"Volume"`
}

// parseShadowCopies turns the CIM response into the facts every platform
// produces. It carries no build constraint so recorded Windows output can be
// tested on any machine.
func parseShadowCopies(raw []byte) (backupFacts, error) {
	rows, err := cim.Unmarshal[windowsBackup](raw)
	if err != nil {
		return backupFacts{}, fmt.Errorf("backup: decode shadow copies: %w", err)
	}

	facts := backupFacts{Supported: true, Mechanism: "Volume Shadow Copy"}
	if len(rows) == 0 {
		return facts, nil
	}
	row := rows[0]

	facts.Configured = row.Configured
	if !facts.Configured {
		return facts, nil
	}

	facts.Last = cim.ParseTime(row.InstallDate)

	// A shadow copy's VolumeName is a device path such as
	// \\?\Volume{GUID}\. It identifies nothing about the person, but it is
	// noise in a report, so only a drive letter is kept.
	if letter := driveLetter(row.Volume); letter != "" {
		facts.Destination = letter
	}
	return facts, nil
}

// driveLetter keeps a plain "C:" and discards a volume GUID path.
func driveLetter(volume string) string {
	volume = strings.TrimSpace(volume)
	if len(volume) >= 2 && volume[1] == ':' {
		return volume[:2]
	}
	return ""
}
