package performance

import (
	"fmt"

	"github.com/ZanOzair/SupportOne/internal/checks/cim"
)

// windowsLoad mirrors the object queryLoad builds. Sizes arrive in the units
// CIM uses: memory in kibibytes, the page file in mebibytes.
type windowsLoad struct {
	Cores       cim.Int  `json:"Cores"`
	BusyPercent cim.Int  `json:"BusyPercent"`
	TotalKB     cim.Uint `json:"TotalKB"`
	FreeKB      cim.Uint `json:"FreeKB"`
	PageTotalMB cim.Uint `json:"PageTotalMB"`
	PageUsedMB  cim.Uint `json:"PageUsedMB"`
}

// parseWindowsLoad turns the CIM response into the same facts every platform
// produces. It lives in a file with no build constraint so recorded Windows
// output can be tested on any machine.
func parseWindowsLoad(raw []byte) (loadFacts, error) {
	rows, err := cim.Unmarshal[windowsLoad](raw)
	if err != nil {
		return loadFacts{}, fmt.Errorf("performance: decode load: %w", err)
	}
	if len(rows) == 0 {
		return loadFacts{}, fmt.Errorf("performance: Windows reported no load figures")
	}
	row := rows[0]

	facts := loadFacts{
		Cores:             int(row.Cores),
		MemTotalBytes:     uint64(row.TotalKB) * 1024,
		MemAvailableBytes: uint64(row.FreeKB) * 1024,
		SwapTotalBytes:    uint64(row.PageTotalMB) * 1024 * 1024,
		SwapUsedBytes:     uint64(row.PageUsedMB) * 1024 * 1024,
	}

	// A machine idle at the instant of the query reports 0, which is a real
	// reading rather than a missing one — so the presence flag is set
	// whenever the query returned anything at all.
	facts.BusyPercent, facts.HasBusy = int(row.BusyPercent), true
	return facts, nil
}
