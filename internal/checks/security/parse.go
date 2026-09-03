package security

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/ZanOzair/supportone/internal/checks/cim"
)

// parseFDESetup reads `fdesetup status`, which prints either
// "FileVault is On." or "FileVault is Off."
func parseFDESetup(data []byte) state {
	s := strings.ToLower(string(data))
	switch {
	case strings.Contains(s, "filevault is on"):
		return stateOn
	case strings.Contains(s, "filevault is off"):
		return stateOff
	default:
		return stateUnknown
	}
}

// parseALFGlobalState reads the macOS application firewall's global state:
// 0 is off, 1 allows signed apps through, 2 blocks all incoming connections.
func parseALFGlobalState(data []byte) state {
	value, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return stateUnknown
	}
	if value == 0 {
		return stateOff
	}
	return stateOn
}

// lsblkDevice mirrors the tree `lsblk -J` prints.
type lsblkDevice struct {
	Name     string        `json:"name"`
	Type     string        `json:"type"`
	FSType   string        `json:"fstype"`
	Children []lsblkDevice `json:"children"`
}

type lsblkReport struct {
	BlockDevices []lsblkDevice `json:"blockdevices"`
}

// parseLUKS reports whether any block device is a LUKS container. That is the
// honest scope of what lsblk can tell us: it says the machine has encrypted
// storage, not that every volume on it is encrypted.
func parseLUKS(data []byte) (state, error) {
	var report lsblkReport
	if err := json.Unmarshal(data, &report); err != nil {
		return stateUnknown, err
	}

	var walk func(devices []lsblkDevice) bool
	walk = func(devices []lsblkDevice) bool {
		for _, d := range devices {
			if strings.EqualFold(d.FSType, "crypto_LUKS") {
				return true
			}
			if walk(d.Children) {
				return true
			}
		}
		return false
	}

	if walk(report.BlockDevices) {
		return stateOn, nil
	}
	return stateOff, nil
}

// winFirewallProfile mirrors what Get-NetFirewallProfile reports per profile.
type winFirewallProfile struct {
	Name    string `json:"Name"`
	Enabled any    `json:"Enabled"`
}

// parseWindowsFirewall reports on only when every profile is on: a machine with
// the public profile disabled is not protected on the network it is most
// exposed to.
func parseWindowsFirewall(data []byte) (state, error) {
	profiles, err := cim.Unmarshal[winFirewallProfile](data)
	if err != nil {
		return stateUnknown, err
	}
	if len(profiles) == 0 {
		return stateUnknown, nil
	}

	for _, p := range profiles {
		switch value := p.Enabled.(type) {
		case bool:
			if !value {
				return stateOff, nil
			}
		case float64:
			if value == 0 {
				return stateOff, nil
			}
		case string:
			if strings.EqualFold(value, "false") || value == "0" {
				return stateOff, nil
			}
		default:
			return stateUnknown, nil
		}
	}
	return stateOn, nil
}

// winAntivirus mirrors the SecurityCenter2 antivirus registration.
type winAntivirus struct {
	DisplayName  string  `json:"displayName"`
	ProductState cim.Int `json:"productState"`
}

// antivirusEnabledBit is the flag inside productState that Windows sets while a
// registered product's real-time protection is running.
const antivirusEnabledBit = 0x1000

// parseWindowsAntivirus reads the registered antivirus products. Windows packs
// several flags into productState; bit 0x1000 of the middle byte is the one
// that says protection is currently on.
func parseWindowsAntivirus(data []byte) (state, string, error) {
	products, err := cim.Unmarshal[winAntivirus](data)
	if err != nil {
		return stateUnknown, "", err
	}
	if len(products) == 0 {
		return stateOff, "", nil
	}

	name := strings.TrimSpace(products[0].DisplayName)
	for _, p := range products {
		if int(p.ProductState)&antivirusEnabledBit != 0 {
			return stateOn, strings.TrimSpace(p.DisplayName), nil
		}
	}
	return stateOff, name, nil
}

// winBitLocker mirrors the protection status Get-BitLockerVolume reports for
// the system drive: 1 means protection is on.
type winBitLocker struct {
	ProtectionStatus any `json:"ProtectionStatus"`
}

func parseWindowsBitLocker(data []byte) (state, error) {
	volumes, err := cim.Unmarshal[winBitLocker](data)
	if err != nil {
		return stateUnknown, err
	}
	if len(volumes) == 0 {
		return stateUnknown, nil
	}

	switch value := volumes[0].ProtectionStatus.(type) {
	case float64:
		if value == 1 {
			return stateOn, nil
		}
		return stateOff, nil
	case string:
		if strings.EqualFold(value, "On") || value == "1" {
			return stateOn, nil
		}
		return stateOff, nil
	default:
		return stateUnknown, nil
	}
}
