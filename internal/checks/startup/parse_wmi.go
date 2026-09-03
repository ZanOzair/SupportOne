package startup

import (
	"strings"

	"github.com/ZanOzair/SupportOne/internal/checks/cim"
)

// winStartupCommand mirrors the fields the check selects from
// Win32_StartupCommand.
type winStartupCommand struct {
	Name     string `json:"Name"`
	Command  string `json:"Command"`
	Location string `json:"Location"`
	User     string `json:"User"`
}

func parseWindowsStartup(data []byte) ([]item, error) {
	entries, err := cim.Unmarshal[winStartupCommand](data)
	if err != nil {
		return nil, err
	}

	out := make([]item, 0, len(entries))
	for _, e := range entries {
		scope := scopeUser
		// Windows reports "Public" for entries that start for everyone.
		if strings.EqualFold(strings.TrimSpace(e.User), "public") {
			scope = scopeSystem
		}
		out = append(out, item{
			Name:     strings.TrimSpace(e.Name),
			Command:  strings.TrimSpace(e.Command),
			Location: strings.TrimSpace(e.Location),
			Scope:    scope,
		})
	}
	return out, nil
}
