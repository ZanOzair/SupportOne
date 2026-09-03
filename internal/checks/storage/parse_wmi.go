package storage

import (
	"strings"

	"github.com/ZanOzair/SupportOne/internal/checks/cim"
)

// win32LogicalDisk mirrors the fields disk.volumes selects from
// Win32_LogicalDisk, restricted to fixed drives.
type win32LogicalDisk struct {
	DeviceID   string   `json:"DeviceID"`
	FileSystem string   `json:"FileSystem"`
	VolumeName string   `json:"VolumeName"`
	Size       cim.Uint `json:"Size"`
	FreeSpace  cim.Uint `json:"FreeSpace"`
}

func parseWindowsVolumes(data []byte) ([]volume, error) {
	entries, err := cim.Unmarshal[win32LogicalDisk](data)
	if err != nil {
		return nil, err
	}

	var out []volume
	for _, e := range entries {
		if e.Size == 0 {
			// An empty card reader slot reports a drive letter and no size.
			continue
		}
		out = append(out, volume{
			Mount:      e.DeviceID,
			Device:     strings.TrimSpace(e.VolumeName),
			Filesystem: e.FileSystem,
			TotalBytes: uint64(e.Size),
			FreeBytes:  uint64(e.FreeSpace),
		})
	}
	return out, nil
}

// win32PhysicalDisk mirrors what Get-PhysicalDisk reports. HealthStatus is a
// string from the cmdlet and an integer when the same class is read through
// CIM, so both are accepted.
type win32PhysicalDisk struct {
	FriendlyName string `json:"FriendlyName"`
	HealthStatus any    `json:"HealthStatus"`
	MediaType    any    `json:"MediaType"`
	SerialNumber string `json:"SerialNumber"`
}

func parseWindowsDisks(data []byte) ([]disk, error) {
	entries, err := cim.Unmarshal[win32PhysicalDisk](data)
	if err != nil {
		return nil, err
	}

	out := make([]disk, 0, len(entries))
	for _, e := range entries {
		out = append(out, disk{
			Name:   firstNonEmpty(strings.TrimSpace(e.FriendlyName), "disk"),
			Model:  strings.TrimSpace(e.FriendlyName),
			Status: windowsHealth(e.HealthStatus),
		})
	}
	return out, nil
}

// windowsHealth maps the Storage Spaces health values onto our verdict.
// Anything unrecognised is unknown rather than assumed healthy.
func windowsHealth(v any) status {
	switch value := v.(type) {
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "healthy":
			return statusHealthy
		case "warning", "unhealthy":
			return statusFailing
		}
	case float64:
		switch int(value) {
		case 0:
			return statusHealthy
		case 1, 2:
			return statusFailing
		}
	}
	return statusUnknown
}

// win32FailurePredict mirrors MSStorageDriver_FailurePredictStatus, which
// exposes the drive's own "I am about to fail" flag. Reading it needs
// administrator rights, so it supplements the health status rather than
// replacing it.
type win32FailurePredict struct {
	InstanceName   string `json:"InstanceName"`
	PredictFailure bool   `json:"PredictFailure"`
}

// applyFailurePredictions downgrades a drive's verdict when Windows' own
// failure prediction says it is about to fail.
func applyFailurePredictions(disks []disk, data []byte) []disk {
	entries, err := cim.Unmarshal[win32FailurePredict](data)
	if err != nil || len(entries) == 0 {
		return disks
	}

	predicted := false
	for _, e := range entries {
		if e.PredictFailure {
			predicted = true
			break
		}
	}
	if !predicted {
		return disks
	}

	// The instance names of this class do not map cleanly onto friendly disk
	// names, so a prediction marks the set rather than guessing which drive.
	for i := range disks {
		if disks[i].Status == statusHealthy || disks[i].Status == statusUnknown {
			disks[i].Status = statusFailing
		}
	}
	return disks
}
