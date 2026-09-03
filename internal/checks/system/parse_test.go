package system

import (
	"testing"
	"time"
)

func TestParseOSRelease(t *testing.T) {
	fixture := []byte(`# /etc/os-release
NAME="Ubuntu"
VERSION="24.04.1 LTS (Noble Numbat)"
ID=ubuntu
VERSION_ID="24.04"

PRETTY_NAME="Ubuntu 24.04.1 LTS"
`)

	fields := parseOSRelease(fixture)
	for key, want := range map[string]string{
		"NAME":        "Ubuntu",
		"VERSION":     "24.04.1 LTS (Noble Numbat)",
		"VERSION_ID":  "24.04",
		"PRETTY_NAME": "Ubuntu 24.04.1 LTS",
	} {
		if got := fields[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if _, ok := fields["#"]; ok {
		t.Error("comment line was parsed as a field")
	}
}

func TestParseUptime(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"typical", "350735.47 234388.90\n", 350735470 * time.Millisecond, false},
		{"zero", "0.00 0.00\n", 0, false},
		{"empty", "", 0, true},
		{"garbage", "not-a-number 1\n", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseUptime([]byte(tc.in))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseUptime: %v", err)
			}
			if got.Round(time.Millisecond) != tc.want {
				t.Errorf("parseUptime = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestParseCPUInfo(t *testing.T) {
	x86 := []byte(`processor	: 0
vendor_id	: GenuineIntel
model name	: Intel(R) Core(TM) i7-10610U CPU @ 1.80GHz

processor	: 1
model name	: Intel(R) Core(TM) i7-10610U CPU @ 1.80GHz
`)
	model, cores := parseCPUInfo(x86)
	if model != "Intel(R) Core(TM) i7-10610U CPU @ 1.80GHz" {
		t.Errorf("model = %q", model)
	}
	if cores != 2 {
		t.Errorf("cores = %d, want 2", cores)
	}

	arm := []byte(`processor	: 0
BogoMIPS	: 108.00
Hardware	: BCM2835
`)
	model, cores = parseCPUInfo(arm)
	if model != "BCM2835" {
		t.Errorf("arm model = %q, want BCM2835", model)
	}
	if cores != 1 {
		t.Errorf("arm cores = %d, want 1", cores)
	}
}

func TestParseMemTotal(t *testing.T) {
	fixture := []byte(`MemTotal:       16316412 kB
MemFree:         2158140 kB
`)
	got, err := parseMemTotal(fixture)
	if err != nil {
		t.Fatalf("parseMemTotal: %v", err)
	}
	if want := uint64(16316412) * 1024; got != want {
		t.Errorf("parseMemTotal = %d, want %d", got, want)
	}

	if _, err := parseMemTotal([]byte("MemFree: 1 kB\n")); err == nil {
		t.Error("expected an error when MemTotal is absent")
	}
}

func TestBatteryHealth(t *testing.T) {
	tests := []struct {
		name         string
		full, design uint64
		want         int
	}{
		{"healthy", 4800, 5000, 96},
		{"worn", 3000, 5000, 60},
		{"above design reads as full", 5100, 5000, 100},
		{"missing design", 4800, 0, 0},
		{"missing full", 0, 5000, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := batteryHealth(tc.full, tc.design); got != tc.want {
				t.Errorf("batteryHealth(%d, %d) = %d, want %d", tc.full, tc.design, got, tc.want)
			}
		})
	}
}

func TestParseWindowsOS(t *testing.T) {
	fixture := []byte(`{"Caption":"Microsoft Windows 11 Pro","Version":"10.0.22631","BuildNumber":"22631",` +
		`"InstallDate":"/Date(1693651200000)/","LastBootUpTime":"/Date(1725282000000)/"}`)

	now := time.UnixMilli(1725282000000).Add(5 * time.Hour)
	facts, err := parseWindowsOS(fixture, now)
	if err != nil {
		t.Fatalf("parseWindowsOS: %v", err)
	}
	if facts.Name != "Microsoft Windows 11 Pro" || facts.Build != "22631" {
		t.Errorf("facts = %+v", facts)
	}
	if facts.Uptime != 5*time.Hour {
		t.Errorf("uptime = %s, want 5h", facts.Uptime)
	}
	if facts.InstallDate.IsZero() {
		t.Error("install date was not parsed")
	}
}

func TestParseWindowsRAMHandlesSingleAndMultipleModules(t *testing.T) {
	single := []byte(`{"Capacity":"8589934592","Speed":3200,"DeviceLocator":"DIMM0"}`)
	array := []byte(`{"MemoryDevices":4}`)

	facts, err := parseWindowsRAM(single, array)
	if err != nil {
		t.Fatalf("parseWindowsRAM: %v", err)
	}
	if facts.TotalBytes != 8589934592 || facts.SlotsUsed != 1 || facts.Slots != 4 || facts.SpeedMHz != 3200 {
		t.Errorf("single module facts = %+v", facts)
	}

	multiple := []byte(`[{"Capacity":8589934592,"Speed":3200,"DeviceLocator":"DIMM0"},` +
		`{"Capacity":8589934592,"Speed":2666,"DeviceLocator":"DIMM1"}]`)
	facts, err = parseWindowsRAM(multiple, array)
	if err != nil {
		t.Fatalf("parseWindowsRAM: %v", err)
	}
	if facts.TotalBytes != 2*8589934592 || facts.SlotsUsed != 2 {
		t.Errorf("multi module facts = %+v", facts)
	}
	if facts.SpeedMHz != 3200 {
		t.Errorf("speed = %d, want the fastest module's 3200", facts.SpeedMHz)
	}
}

func TestParseWindowsRAMFallsBackToSlotsUsed(t *testing.T) {
	modules := []byte(`[{"Capacity":8589934592,"Speed":3200,"DeviceLocator":"DIMM0"},` +
		`{"Capacity":8589934592,"Speed":3200,"DeviceLocator":"DIMM1"}]`)

	facts, err := parseWindowsRAM(modules, []byte(`null`))
	if err != nil {
		t.Fatalf("parseWindowsRAM: %v", err)
	}
	if facts.Slots != 2 {
		t.Errorf("slots = %d, want the used count when the array class reports nothing", facts.Slots)
	}
}

func TestParseWindowsBattery(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		wantPresent bool
		wantHealth  int
	}{
		{"desktop", `{"Present":false}`, false, 0},
		{"laptop with capacities", `{"Present":true,"DesignedCapacity":50000,"FullChargedCapacity":41000,"CycleCount":230}`, true, 82},
		{"laptop without capacities", `{"Present":true,"DesignedCapacity":null,"FullChargedCapacity":null,"CycleCount":0}`, true, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			facts, err := parseWindowsBattery([]byte(tc.in))
			if err != nil {
				t.Fatalf("parseWindowsBattery: %v", err)
			}
			if facts.Present != tc.wantPresent || facts.HealthPercent != tc.wantHealth {
				t.Errorf("facts = %+v, want present=%v health=%d", facts, tc.wantPresent, tc.wantHealth)
			}
		})
	}
}

func TestParseSWVers(t *testing.T) {
	fixture := []byte("ProductName:\t\tmacOS\nProductVersion:\t\t14.5\nBuildVersion:\t\t23F79\n")
	fields := parseSWVers(fixture)
	if fields["ProductName"] != "macOS" || fields["ProductVersion"] != "14.5" || fields["BuildVersion"] != "23F79" {
		t.Errorf("fields = %v", fields)
	}
}

func TestParseBootTime(t *testing.T) {
	fixture := []byte("{ sec = 1725282000, usec = 123456 } Mon Sep  2 13:20:00 2026\n")
	got, err := parseBootTime(fixture)
	if err != nil {
		t.Fatalf("parseBootTime: %v", err)
	}
	if want := time.Unix(1725282000, 0).UTC(); !got.Equal(want) {
		t.Errorf("parseBootTime = %v, want %v", got, want)
	}

	if _, err := parseBootTime([]byte("nothing useful")); err == nil {
		t.Error("expected an error for output with no sec field")
	}
}

func TestParseMacHardware(t *testing.T) {
	appleSilicon := []byte(`{"SPHardwareDataType":[{"machine_name":"MacBook Pro",` +
		`"machine_model":"MacBookPro18,3","chip_type":"Apple M1 Pro","number_processors":"proc 10:8:2"}]}`)
	facts, err := parseMacHardware(appleSilicon)
	if err != nil {
		t.Fatalf("parseMacHardware: %v", err)
	}
	if facts.CPU != "Apple M1 Pro" || facts.Cores != 10 || facts.Model != "MacBook Pro" {
		t.Errorf("apple silicon facts = %+v", facts)
	}

	intel := []byte(`{"SPHardwareDataType":[{"machine_name":"iMac","machine_model":"iMac19,1",` +
		`"cpu_type":"Intel Core i5","number_processors":6}]}`)
	facts, err = parseMacHardware(intel)
	if err != nil {
		t.Fatalf("parseMacHardware: %v", err)
	}
	if facts.CPU != "Intel Core i5" || facts.Cores != 6 {
		t.Errorf("intel facts = %+v", facts)
	}

	if _, err := parseMacHardware([]byte(`{"SPHardwareDataType":[]}`)); err == nil {
		t.Error("expected an error when system_profiler reports nothing")
	}
}

func TestParseMacBattery(t *testing.T) {
	laptop := []byte(`{"SPPowerDataType":[{"_name":"spbattery_information",` +
		`"sppower_battery_health_info":{"sppower_battery_cycle_count":412,` +
		`"sppower_battery_health":"Normal","sppower_battery_health_maximum_capacity":"88%"}}]}`)
	facts, err := parseMacBattery(laptop)
	if err != nil {
		t.Fatalf("parseMacBattery: %v", err)
	}
	if !facts.Present || facts.HealthPercent != 88 || facts.CycleCount != 412 {
		t.Errorf("laptop facts = %+v", facts)
	}

	desktop := []byte(`{"SPPowerDataType":[{"_name":"sppower_information"}]}`)
	facts, err = parseMacBattery(desktop)
	if err != nil {
		t.Fatalf("parseMacBattery: %v", err)
	}
	if facts.Present {
		t.Errorf("desktop reported a battery: %+v", facts)
	}
}

func TestParsePercent(t *testing.T) {
	tests := []struct {
		in     string
		want   int
		wantOK bool
	}{
		{"88%", 88, true},
		{" 100 % ", 100, true},
		{"100%", 100, true},
		{"", 0, false},
		{"unknown", 0, false},
		{"0%", 0, false},
	}
	for _, tc := range tests {
		got, ok := parsePercent(tc.in)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("parsePercent(%q) = (%d, %v), want (%d, %v)", tc.in, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestParseWindowsHardware(t *testing.T) {
	fixture := []byte(`{"Manufacturer":"LENOVO","Model":"20XW00AAUK","Cores":8,` +
		`"Cpu":"11th Gen Intel(R) Core(TM) i7-1185G7 @ 3.00GHz","Memory":"34093780992"}`)

	facts, err := parseWindowsHardware(fixture)
	if err != nil {
		t.Fatalf("parseWindowsHardware: %v", err)
	}
	if facts.Vendor != "LENOVO" || facts.Model != "20XW00AAUK" || facts.Cores != 8 {
		t.Errorf("facts = %+v", facts)
	}
	if facts.CPU != "11th Gen Intel(R) Core(TM) i7-1185G7 @ 3.00GHz" {
		t.Errorf("cpu = %q", facts.CPU)
	}

	if _, err := parseWindowsHardware([]byte(`null`)); err == nil {
		t.Error("expected an error when Win32_ComputerSystem returns nothing")
	}
}
