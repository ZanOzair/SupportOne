package storage

import (
	"encoding/json"
	"fmt"
	"strings"
)

// reallocatedSectorID is the SMART attribute that counts sectors the drive has
// retired because they went bad — the number behind "your disk has bad spots".
const reallocatedSectorID = 5

// smartctlReport mirrors the subset of `smartctl --json` output the check reads.
type smartctlReport struct {
	Device struct {
		Name string `json:"name"`
	} `json:"device"`
	ModelName   string `json:"model_name"`
	SMARTStatus struct {
		Passed *bool `json:"passed"`
	} `json:"smart_status"`
	ATAAttributes struct {
		Table []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
			Raw  struct {
				Value int `json:"value"`
			} `json:"raw"`
		} `json:"table"`
	} `json:"ata_smart_attributes"`
}

// parseSmartctl turns one drive's smartctl report into a disk record.
//
// A drive that reports no verdict is recorded as unknown, never as healthy:
// USB enclosures and some NVMe bridges pass no SMART data through at all.
func parseSmartctl(data []byte, fallbackName string) (disk, error) {
	var report smartctlReport
	if err := json.Unmarshal(data, &report); err != nil {
		return disk{}, fmt.Errorf("storage: parse smartctl output: %w", err)
	}

	d := disk{
		Name:   firstNonEmpty(report.Device.Name, fallbackName),
		Model:  strings.TrimSpace(report.ModelName),
		Status: statusUnknown,
	}
	if report.SMARTStatus.Passed != nil {
		if *report.SMARTStatus.Passed {
			d.Status = statusHealthy
		} else {
			d.Status = statusFailing
		}
	}

	for _, attr := range report.ATAAttributes.Table {
		if attr.ID == reallocatedSectorID {
			count := attr.Raw.Value
			d.Reallocated = &count
			break
		}
	}
	return d, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
