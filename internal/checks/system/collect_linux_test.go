package system

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/platform"
)

// fixtureMachine writes a recorded /proc, /sys and /etc for one machine and
// points the collectors at it, so the Linux collection path is exercised
// without depending on whatever the test machine happens to be.
func fixtureMachine(t *testing.T, files map[string]string) {
	t.Helper()

	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	oldProc, oldSys, oldEtc := procRoot, sysRoot, etcRoot
	procRoot = filepath.Join(root, "proc")
	sysRoot = filepath.Join(root, "sys")
	etcRoot = filepath.Join(root, "etc")
	t.Cleanup(func() { procRoot, sysRoot, etcRoot = oldProc, oldSys, oldEtc })
}

const (
	osRelease = "NAME=\"Ubuntu\"\nVERSION=\"24.04.1 LTS (Noble Numbat)\"\nVERSION_ID=\"24.04\"\n"
	cpuInfo   = "processor\t: 0\nmodel name\t: Intel(R) Core(TM) i7-10610U CPU @ 1.80GHz\n\n" +
		"processor\t: 1\nmodel name\t: Intel(R) Core(TM) i7-10610U CPU @ 1.80GHz\n"
)

func TestCollectOSFromFixtureTree(t *testing.T) {
	fixtureMachine(t, map[string]string{
		"etc/os-release":            osRelease,
		"proc/uptime":               "185000.12 900000.00\n",
		"proc/sys/kernel/osrelease": "6.8.0-45-generic\n",
	})

	facts, err := collectOS(context.Background(), nil)
	if err != nil {
		t.Fatalf("collectOS: %v", err)
	}
	if facts.Name != "Ubuntu" || facts.Version != "24.04.1 LTS (Noble Numbat)" {
		t.Errorf("facts = %+v", facts)
	}
	if facts.Kernel != "6.8.0-45-generic" {
		t.Errorf("kernel = %q", facts.Kernel)
	}
	if facts.Uptime.Hours() < 51 || facts.Uptime.Hours() > 52 {
		t.Errorf("uptime = %s, want about 51h", facts.Uptime)
	}
	// Linux records no install date, and a guess would be worse than nothing.
	if !facts.InstallDate.IsZero() {
		t.Errorf("install date = %v, want zero", facts.InstallDate)
	}
}

func TestCollectHardwareReportsMissingDMIRatherThanGuessing(t *testing.T) {
	fixtureMachine(t, map[string]string{"proc/cpuinfo": cpuInfo})

	facts, err := collectHardware(context.Background(), nil)
	if err != nil {
		t.Fatalf("collectHardware: %v", err)
	}
	if facts.Cores != 2 {
		t.Errorf("cores = %d, want 2", facts.Cores)
	}
	if facts.Model != "" || facts.Vendor != "" {
		t.Errorf("facts = %+v, want an empty model where there is no DMI data", facts)
	}

	// And the check turns that into a sentence saying so, still OK.
	res, err := hardwareInventoryCheck{}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Severity != checks.SeverityOK || res.Summary != keyHardwareUnreported {
		t.Errorf("result = %+v, want the unreported-model message", res)
	}
}

func TestCollectHardwareReadsDMIWhenPresent(t *testing.T) {
	fixtureMachine(t, map[string]string{
		"proc/cpuinfo":                  cpuInfo,
		"sys/class/dmi/id/sys_vendor":   "LENOVO\n",
		"sys/class/dmi/id/product_name": "20XW00AAUK\n",
	})

	facts, err := collectHardware(context.Background(), nil)
	if err != nil {
		t.Fatalf("collectHardware: %v", err)
	}
	if facts.Vendor != "LENOVO" || facts.Model != "20XW00AAUK" {
		t.Errorf("facts = %+v", facts)
	}
}

func TestRAMCheckFlagsAMemoryStarvedMachine(t *testing.T) {
	fixtureMachine(t, map[string]string{"proc/meminfo": "MemTotal:        2048000 kB\n"})

	res, err := ramCheck{}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Severity != checks.SeverityAttention {
		t.Errorf("severity = %q, want attention below 4 GiB", res.Severity)
	}

	fixtureMachine(t, map[string]string{"proc/meminfo": "MemTotal:       16316412 kB\n"})
	if res, err = (ramCheck{}).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Severity != checks.SeverityOK {
		t.Errorf("severity = %q, want ok at 15.6 GiB", res.Severity)
	}
}

func TestBatteryCheckOnADesktopAndALaptop(t *testing.T) {
	// No power supply class at all: a desktop.
	fixtureMachine(t, map[string]string{"proc/cpuinfo": cpuInfo})
	res, err := batteryCheck{}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Severity != checks.SeverityOK || res.Summary != keyBatteryAbsent {
		t.Errorf("desktop result = %+v, want a plain 'no battery'", res)
	}

	// A worn laptop battery, reported through the energy_* pair.
	fixtureMachine(t, map[string]string{
		"sys/class/power_supply/AC/type":                 "Mains\n",
		"sys/class/power_supply/BAT0/type":               "Battery\n",
		"sys/class/power_supply/BAT0/energy_full":        "32000000\n",
		"sys/class/power_supply/BAT0/energy_full_design": "57000000\n",
		"sys/class/power_supply/BAT0/cycle_count":        "412\n",
	})
	if res, err = (batteryCheck{}).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Severity != checks.SeverityAttention {
		t.Errorf("severity = %q, want attention at 56%% of design capacity", res.Severity)
	}
	if res.Detail["cycle_count"] != 412 {
		t.Errorf("cycle count = %v, want 412", res.Detail["cycle_count"])
	}
}

func TestBatteryCheckReadsTheChargePairToo(t *testing.T) {
	// Kernels expose either energy_* or charge_*; both give the same ratio.
	fixtureMachine(t, map[string]string{
		"sys/class/power_supply/BAT0/type":               "Battery\n",
		"sys/class/power_supply/BAT0/charge_full":        "4800000\n",
		"sys/class/power_supply/BAT0/charge_full_design": "5000000\n",
	})

	res, err := batteryCheck{}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Severity != checks.SeverityOK || res.Detail["health_percent"] != 96 {
		t.Errorf("result = %+v, want a healthy 96%%", res)
	}
}

func TestBatteryPresentButUnreadableIsNotHealthy(t *testing.T) {
	fixtureMachine(t, map[string]string{
		"sys/class/power_supply/BAT0/type": "Battery\n",
	})

	res, err := batteryCheck{}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Severity != checks.SeverityUnknown {
		t.Errorf("severity = %q, want unknown — an unreadable battery is not a healthy one", res.Severity)
	}
}

func TestOSCheckReportsUnknownWhenTheTreeIsMissing(t *testing.T) {
	fixtureMachine(t, map[string]string{})

	res, err := osInfoCheck{}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Severity != checks.SeverityUnknown || res.Err == "" {
		t.Errorf("result = %+v, want unknown with the reason recorded", res)
	}
}

func TestChecksDeclareEveryPlatform(t *testing.T) {
	for _, c := range []checks.Check{
		osInfoCheck{}, hardwareInventoryCheck{}, ramCheck{}, batteryCheck{},
	} {
		if len(c.Platforms()) != len(platform.All()) {
			t.Errorf("%s runs on %v, want every platform", c.ID(), c.Platforms())
		}
		if c.RequiresAdmin() {
			t.Errorf("%s asks for administrator rights it does not need", c.ID())
		}
	}
}
