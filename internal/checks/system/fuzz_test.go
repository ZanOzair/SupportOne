package system

import (
	"testing"
	"time"
)

// The parsers read whatever the operating system's tools printed. That output
// is not attacker-controlled in the usual sense, but it is not this program's
// either: it changes between OS versions, it is localised, it gets truncated
// when a command is killed, and it arrives as bytes from a pipe. A parser that
// panics on any of that takes the whole snapshot down with it, so the property
// worth fuzzing is simply that every one of them returns.

func FuzzParseOSRelease(f *testing.F) {
	f.Add([]byte("NAME=\"Ubuntu\"\nVERSION_ID=\"24.04\"\n"))
	f.Add([]byte("NAME=\nVERSION_ID"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, data []byte) {
		for key, value := range parseOSRelease(data) {
			if key == "" {
				t.Errorf("parsed an empty key with value %q", value)
			}
		}
	})
}

func FuzzParseUptime(f *testing.F) {
	f.Add([]byte("3600.50 7200.00"))
	f.Add([]byte("-1"))
	f.Add([]byte("99999999999999999999999999"))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseUptime(data)
	})
}

func FuzzParseCPUInfo(f *testing.F) {
	f.Add([]byte("model name\t: Intel Core i7\nprocessor\t: 0\nprocessor\t: 1\n"))
	f.Add([]byte("model name:"))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, cores := parseCPUInfo(data)
		if cores < 0 {
			t.Errorf("negative core count %d", cores)
		}
	})
}

func FuzzParseMemTotal(f *testing.F) {
	f.Add([]byte("MemTotal:       16384000 kB\n"))
	f.Add([]byte("MemTotal: notanumber kB"))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseMemTotal(data)
		_, _ = parseSysfsUint(data)
	})
}

func FuzzParseSWVers(f *testing.F) {
	f.Add([]byte("ProductName:\tmacOS\nProductVersion:\t14.5\n"))
	f.Add([]byte(":::::"))

	f.Fuzz(func(t *testing.T, data []byte) {
		_ = parseSWVers(data)
		_, _ = parseBootTime(data)
		_, _ = parseMacHardware(data)
		_, _ = parseMacBattery(data)
	})
}

func FuzzParseWindowsSystem(f *testing.F) {
	f.Add([]byte(`{"Caption":"Windows 11","Version":"10.0.22631"}`))
	f.Add([]byte(`{"Caption":`))
	f.Add([]byte(`[]`))

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseWindowsOS(data, now)
		_, _ = parseWindowsHardware(data)
		_, _ = parseWindowsRAM(data, data)
		_, _ = parseWindowsBattery(data)
	})
}

func FuzzParsePercent(f *testing.F) {
	f.Add("87%")
	f.Add("%")
	f.Add("999999999999999999999999")

	f.Fuzz(func(t *testing.T, s string) {
		value, ok := parsePercent(s)
		if ok && (value < 0 || value > 100) {
			t.Errorf("parsePercent(%q) = %d, which is not a percentage", s, value)
		}
	})
}
