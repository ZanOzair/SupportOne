package system

import (
	"fmt"
	"strings"
	"time"

	"github.com/ZanOzair/supportone/internal/checks/cim"
)

// win32OperatingSystem mirrors the fields os.info selects from
// Win32_OperatingSystem.
type win32OperatingSystem struct {
	Caption        string `json:"Caption"`
	Version        string `json:"Version"`
	BuildNumber    string `json:"BuildNumber"`
	InstallDate    string `json:"InstallDate"`
	LastBootUpTime string `json:"LastBootUpTime"`
}

func parseWindowsOS(data []byte, now time.Time) (osFacts, error) {
	entries, err := cim.Unmarshal[win32OperatingSystem](data)
	if err != nil {
		return osFacts{}, err
	}
	if len(entries) == 0 {
		return osFacts{}, fmt.Errorf("system: Win32_OperatingSystem returned nothing")
	}
	os := entries[0]

	facts := osFacts{
		Name:        strings.TrimSpace(os.Caption),
		Version:     strings.TrimSpace(os.Version),
		Build:       strings.TrimSpace(os.BuildNumber),
		Kernel:      strings.TrimSpace(os.Version),
		InstallDate: cim.ParseTime(os.InstallDate),
	}
	if boot := cim.ParseTime(os.LastBootUpTime); !boot.IsZero() {
		facts.Uptime = now.Sub(boot)
	}
	return facts, nil
}

// win32Hardware mirrors the composite object the hardware.inventory command
// builds from Win32_ComputerSystem and Win32_Processor.
type win32Hardware struct {
	Manufacturer string   `json:"Manufacturer"`
	Model        string   `json:"Model"`
	Cores        int      `json:"Cores"`
	CPU          string   `json:"Cpu"`
	Memory       cim.Uint `json:"Memory"`
}

func parseWindowsHardware(data []byte) (hardwareFacts, error) {
	entries, err := cim.Unmarshal[win32Hardware](data)
	if err != nil {
		return hardwareFacts{}, err
	}
	if len(entries) == 0 {
		return hardwareFacts{}, fmt.Errorf("system: Win32_ComputerSystem returned nothing")
	}
	hw := entries[0]

	return hardwareFacts{
		Vendor: strings.TrimSpace(hw.Manufacturer),
		Model:  strings.TrimSpace(hw.Model),
		CPU:    strings.TrimSpace(hw.CPU),
		Cores:  hw.Cores,
	}, nil
}

// win32PhysicalMemory mirrors one installed memory module.
type win32PhysicalMemory struct {
	Capacity      cim.Uint `json:"Capacity"`
	Speed         int      `json:"Speed"`
	DeviceLocator string   `json:"DeviceLocator"`
}

// win32MemoryArray reports how many slots the board has.
type win32MemoryArray struct {
	MemoryDevices int `json:"MemoryDevices"`
}

func parseWindowsRAM(modulesJSON, arrayJSON []byte) (ramFacts, error) {
	modules, err := cim.Unmarshal[win32PhysicalMemory](modulesJSON)
	if err != nil {
		return ramFacts{}, err
	}

	var facts ramFacts
	for _, m := range modules {
		facts.TotalBytes += uint64(m.Capacity)
		if m.Capacity > 0 {
			facts.SlotsUsed++
		}
		if m.Speed > facts.SpeedMHz {
			facts.SpeedMHz = m.Speed
		}
	}

	if arrays, err := cim.Unmarshal[win32MemoryArray](arrayJSON); err == nil && len(arrays) > 0 {
		facts.Slots = arrays[0].MemoryDevices
	}
	if facts.Slots < facts.SlotsUsed {
		facts.Slots = facts.SlotsUsed
	}
	return facts, nil
}

// win32Battery mirrors the composite object the battery command builds. Windows
// reports capacities through several classes and any of them may be null, so
// every field is optional and a missing pair yields "unreadable".
type win32Battery struct {
	Present       bool     `json:"Present"`
	DesignedMWh   cim.Uint `json:"DesignedCapacity"`
	FullChargeMWh cim.Uint `json:"FullChargedCapacity"`
	CycleCount    int      `json:"CycleCount"`
}

func parseWindowsBattery(data []byte) (batteryFacts, error) {
	entries, err := cim.Unmarshal[win32Battery](data)
	if err != nil {
		return batteryFacts{}, err
	}
	if len(entries) == 0 {
		return batteryFacts{Present: false}, nil
	}
	b := entries[0]
	if !b.Present {
		return batteryFacts{Present: false}, nil
	}

	return batteryFacts{
		Present:       true,
		CycleCount:    b.CycleCount,
		HealthPercent: batteryHealth(uint64(b.FullChargeMWh), uint64(b.DesignedMWh)),
	}, nil
}
