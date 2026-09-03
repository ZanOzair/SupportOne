package storage

import (
	"context"

	"github.com/ZanOzair/supportone/internal/platform"
)

// Compiled-in queries; see platform.RunRead for why this is not shell
// construction.
const (
	psExe = "powershell"

	// DriveType 3 is a fixed local disk: not a network share, not a CD.
	queryVolumes = `Get-CimInstance Win32_LogicalDisk -Filter "DriveType=3" | ` +
		`Select-Object DeviceID,FileSystem,VolumeName,Size,FreeSpace | ConvertTo-Json -Compress`

	queryPhysicalDisks = `Get-PhysicalDisk | Select-Object FriendlyName,HealthStatus,MediaType,SerialNumber | ConvertTo-Json -Compress`

	// Reading the drive's own failure prediction needs administrator rights;
	// SilentlyContinue keeps its absence from failing the whole check.
	queryFailurePredict = `Get-CimInstance -Namespace root\wmi -ClassName MSStorageDriver_FailurePredictStatus ` +
		`-ErrorAction SilentlyContinue | Select-Object InstanceName,PredictFailure | ConvertTo-Json -Compress`
)

func psArgs(query string) []string {
	return []string{"-NoProfile", "-NonInteractive", "-Command", query}
}

func collectVolumes(ctx context.Context, run platform.Runner) ([]volume, error) {
	out, err := run(ctx, psExe, psArgs(queryVolumes)...)
	if err != nil {
		return nil, err
	}
	return parseWindowsVolumes(out)
}

func collectDisks(ctx context.Context, run platform.Runner) ([]disk, error) {
	out, err := run(ctx, psExe, psArgs(queryPhysicalDisks)...)
	if err != nil {
		return nil, err
	}
	disks, err := parseWindowsDisks(out)
	if err != nil {
		return nil, err
	}

	// The prediction flag is a bonus: without elevation it returns nothing and
	// the health status stands on its own.
	if predictions, predErr := run(ctx, psExe, psArgs(queryFailurePredict)...); predErr == nil {
		disks = applyFailurePredictions(disks, predictions)
	}
	return disks, nil
}
