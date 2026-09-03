package drivers

import (
	"strings"

	"github.com/ZanOzair/SupportOne/internal/checks/cim"
)

// winPnPEntity mirrors the fields the check selects from Win32_PnPEntity.
type winPnPEntity struct {
	Name                   string  `json:"Name"`
	DeviceID               string  `json:"DeviceID"`
	ConfigManagerErrorCode cim.Int `json:"ConfigManagerErrorCode"`
}

// errorMeanings translates the Configuration Manager codes a user is most
// likely to hit into what they actually mean. Codes without an entry are
// reported by number rather than described inaccurately.
var errorMeanings = map[int]string{
	1:  "not configured correctly",
	3:  "the driver may be corrupted, or the system is low on memory",
	10: "cannot start",
	12: "cannot find enough free resources to use",
	14: "needs the computer to be restarted",
	18: "the drivers need to be reinstalled",
	22: "disabled",
	28: "the drivers are not installed",
	31: "Windows cannot load the drivers for this device",
	43: "Windows stopped this device because it reported problems",
	45: "not currently connected to the computer",
}

func parseProblemDevices(data []byte) ([]device, error) {
	entries, err := cim.Unmarshal[winPnPEntity](data)
	if err != nil {
		return nil, err
	}

	var out []device
	for _, e := range entries {
		code := int(e.ConfigManagerErrorCode)
		if code == 0 {
			continue
		}
		// Code 45 means a device that is simply unplugged — a USB dock that
		// went home with its owner is not a driver fault.
		if code == 45 {
			continue
		}

		out = append(out, device{
			Name:      strings.TrimSpace(e.Name),
			DeviceID:  strings.TrimSpace(e.DeviceID),
			ErrorCode: code,
			Meaning:   errorMeanings[code],
		})
	}
	return out, nil
}
