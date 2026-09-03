package system

import (
	"context"
	"time"

	"github.com/ZanOzair/SupportOne/internal/checks/cim"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

// The PowerShell invocations below are compiled-in constants. Nothing in them
// is assembled from user input, check output or model output, and no shell
// interprets them: each is a single argv entry passed to the executable.
const (
	psExe = "powershell"

	queryOS = `Get-CimInstance Win32_OperatingSystem | ` +
		`Select-Object Caption,Version,BuildNumber,InstallDate,LastBootUpTime | ConvertTo-Json -Compress`

	queryHardware = `$cs = Get-CimInstance Win32_ComputerSystem; ` +
		`$cpu = Get-CimInstance Win32_Processor | Select-Object -First 1; ` +
		`[pscustomobject]@{Manufacturer=$cs.Manufacturer; Model=$cs.Model; ` +
		`Cores=$cs.NumberOfLogicalProcessors; Cpu=$cpu.Name; Memory=$cs.TotalPhysicalMemory} | ConvertTo-Json -Compress`

	queryMemoryModules = `Get-CimInstance Win32_PhysicalMemory | ` +
		`Select-Object Capacity,Speed,DeviceLocator | ConvertTo-Json -Compress`

	queryMemoryArray = `Get-CimInstance Win32_PhysicalMemoryArray | Select-Object MemoryDevices | ConvertTo-Json -Compress`

	// Capacities live in different classes and any of them can be null, so the
	// query gathers what it can and the parser decides whether that is enough.
	queryBattery = `$b = Get-CimInstance Win32_Battery | Select-Object -First 1; ` +
		`if ($null -eq $b) { [pscustomobject]@{Present=$false} | ConvertTo-Json -Compress } else { ` +
		`$static = Get-CimInstance -Namespace root\wmi -ClassName BatteryStaticData -ErrorAction SilentlyContinue | Select-Object -First 1; ` +
		`$full = Get-CimInstance -Namespace root\wmi -ClassName BatteryFullChargedCapacity -ErrorAction SilentlyContinue | Select-Object -First 1; ` +
		`$cycles = Get-CimInstance -Namespace root\wmi -ClassName BatteryCycleCount -ErrorAction SilentlyContinue | Select-Object -First 1; ` +
		`[pscustomobject]@{Present=$true; DesignedCapacity=$static.DesignedCapacity; ` +
		`FullChargedCapacity=$full.FullChargedCapacity; CycleCount=$cycles.CycleCount} | ConvertTo-Json -Compress }`
)

// psArgs wraps a compiled-in query in the flags that keep PowerShell from
// loading a user profile or waiting for input.
func psArgs(query string) []string {
	return []string{"-NoProfile", "-NonInteractive", "-Command", query}
}

func collectOS(ctx context.Context, run platform.Runner) (osFacts, error) {
	out, err := run(ctx, psExe, psArgs(queryOS)...)
	if err != nil {
		return osFacts{}, err
	}
	return parseWindowsOS(out, time.Now())
}

func collectHardware(ctx context.Context, run platform.Runner) (hardwareFacts, error) {
	out, err := run(ctx, psExe, psArgs(queryHardware)...)
	if err != nil {
		return hardwareFacts{}, err
	}
	return parseWindowsHardware(out)
}

func collectRAM(ctx context.Context, run platform.Runner) (ramFacts, error) {
	modules, err := run(ctx, psExe, psArgs(queryMemoryModules)...)
	if err != nil {
		return ramFacts{}, err
	}

	// Slot count is a nicety; losing it should not cost the memory total.
	array, err := run(ctx, psExe, psArgs(queryMemoryArray)...)
	if err != nil {
		array = nil
	}

	facts, err := parseWindowsRAM(modules, array)
	if err != nil {
		return ramFacts{}, err
	}
	if facts.TotalBytes == 0 {
		// Win32_PhysicalMemory is empty on some virtual machines; fall back to
		// the total the computer system reports.
		if hw, hwErr := run(ctx, psExe, psArgs(queryHardware)...); hwErr == nil {
			if entries, parseErr := cim.Unmarshal[win32Hardware](hw); parseErr == nil && len(entries) > 0 {
				facts.TotalBytes = uint64(entries[0].Memory)
			}
		}
	}
	return facts, nil
}

func collectBattery(ctx context.Context, run platform.Runner) (batteryFacts, error) {
	out, err := run(ctx, psExe, psArgs(queryBattery)...)
	if err != nil {
		return batteryFacts{}, err
	}
	return parseWindowsBattery(out)
}
