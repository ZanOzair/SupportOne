package performance

import "testing"

// Load and memory figures feed arithmetic, so a parser that returns a wrong
// number is worse here than one that returns nothing. The invariants below say
// what "nothing" has to look like.

func FuzzParseLoadAvg(f *testing.F) {
	f.Add([]byte("0.52 0.58 0.59 1/523 12345"))
	f.Add([]byte("notanumber"))
	f.Add([]byte("-5"))

	f.Fuzz(func(t *testing.T, data []byte) {
		load, ok := parseLoadAvg(data)
		if ok && load < 0 {
			t.Errorf("parseLoadAvg(%q) = %v, which is not a load average", data, load)
		}
	})
}

func FuzzParseMemInfo(f *testing.F) {
	f.Add([]byte("MemTotal: 16384000 kB\nMemAvailable: 8192000 kB\n"))
	f.Add([]byte("MemTotal:"))

	f.Fuzz(func(t *testing.T, data []byte) {
		for key := range parseMemInfo(data) {
			if key == "" {
				t.Error("parsed an empty key out of meminfo")
			}
		}
	})
}

func FuzzParseMacPerformance(f *testing.F) {
	f.Add([]byte("vm.loadavg: { 1.50 1.20 1.10 }"))
	f.Add([]byte("Pages free: 123456."))
	f.Add([]byte("total = 2048.00M  used = 1024.00M"))

	f.Fuzz(func(t *testing.T, data []byte) {
		load, ok := parseSysctlLoadAvg(data)
		if ok && load < 0 {
			t.Errorf("parseSysctlLoadAvg(%q) = %v", data, load)
		}
		_ = parseVMStatAvailable(data)
		_, _ = parseSwapUsage(data)
	})
}

func FuzzParseSizeSuffix(f *testing.F) {
	f.Add("2048.00M")
	f.Add("M")
	f.Add("999999999999999999999999G")

	f.Fuzz(func(t *testing.T, s string) {
		_ = parseSizeSuffix(s)
		_ = parsePageSize(s)
	})
}

func FuzzParseWindowsLoad(f *testing.F) {
	f.Add([]byte(`{"LoadPercentage":50,"TotalVisibleMemorySize":16000000}`))
	f.Add([]byte(`{"LoadPercentage":`))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseWindowsLoad(data)
	})
}
