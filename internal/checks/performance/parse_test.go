package performance

import "testing"

func TestParseLoadAvg(t *testing.T) {
	cases := []struct {
		raw   string
		want  float64
		valid bool
	}{
		{"0.52 0.58 0.59 1/1234 5678\n", 0.52, true},
		{"12.00 8.31 4.02 9/2201 31337", 12.00, true},
		{"0.00 0.00 0.00 1/100 200", 0, true},
		{"", 0, false},
		{"not-a-number 0.5 0.5", 0, false},
	}

	for _, tc := range cases {
		got, ok := parseLoadAvg([]byte(tc.raw))
		if ok != tc.valid {
			t.Errorf("parseLoadAvg(%q) ok = %v, want %v", tc.raw, ok, tc.valid)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("parseLoadAvg(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

func TestParseMemInfo(t *testing.T) {
	raw := `MemTotal:       16324812 kB
MemFree:          812344 kB
MemAvailable:    9218044 kB
Buffers:          201344 kB
SwapTotal:       2097148 kB
SwapFree:        2097148 kB
HugePages_Total:       0
`

	got := parseMemInfo([]byte(raw))

	// The kernel writes kibibytes and labels them kB. The conversion is the
	// whole reason this parser exists.
	if got["MemTotal"] != 16324812*1024 {
		t.Errorf("MemTotal = %d, want %d", got["MemTotal"], uint64(16324812)*1024)
	}
	if got["MemAvailable"] != 9218044*1024 {
		t.Errorf("MemAvailable = %d", got["MemAvailable"])
	}
	if got["SwapTotal"] != 2097148*1024 {
		t.Errorf("SwapTotal = %d", got["SwapTotal"])
	}
	// A value with no unit is already a count, not a size.
	if got["HugePages_Total"] != 0 {
		t.Errorf("HugePages_Total = %d, want 0", got["HugePages_Total"])
	}
	if _, ok := got["NotThere"]; ok {
		t.Error("parseMemInfo invented a field")
	}
}

func TestParseSysctlLoadAvg(t *testing.T) {
	cases := []struct {
		raw   string
		want  float64
		valid bool
	}{
		{"{ 1.85 1.95 2.01 }\n", 1.85, true},
		{"{ 0.00 0.00 0.00 }", 0, true},
		{"", 0, false},
		{"{ }", 0, false},
	}

	for _, tc := range cases {
		got, ok := parseSysctlLoadAvg([]byte(tc.raw))
		if ok != tc.valid {
			t.Errorf("parseSysctlLoadAvg(%q) ok = %v, want %v", tc.raw, ok, tc.valid)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("parseSysctlLoadAvg(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

func TestParseVMStatAvailable(t *testing.T) {
	raw := `Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                              100000.
Pages active:                            300000.
Pages inactive:                           50000.
Pages speculative:                        20000.
Pages wired down:                        150000.
`

	// Free plus inactive plus speculative: the pages macOS would hand to a
	// new process without swapping.
	want := uint64(100000+50000+20000) * 16384
	if got := parseVMStatAvailable([]byte(raw)); got != want {
		t.Errorf("parseVMStatAvailable = %d, want %d", got, want)
	}

	if got := parseVMStatAvailable([]byte("")); got != 0 {
		t.Errorf("empty output = %d, want 0", got)
	}
	if got := parseVMStatAvailable([]byte("something else entirely\nPages free: 100.\n")); got != 0 {
		t.Errorf("output with no page size = %d, want 0", got)
	}
}

func TestParseSwapUsage(t *testing.T) {
	total, used := parseSwapUsage([]byte("total = 2048.00M  used = 512.00M  free = 1536.00M  (encrypted)\n"))
	if total != 2048*(1<<20) {
		t.Errorf("total = %d, want %d", total, 2048*(1<<20))
	}
	if used != 512*(1<<20) {
		t.Errorf("used = %d, want %d", used, 512*(1<<20))
	}

	// A machine with swap disabled reports zeros, not an error.
	total, used = parseSwapUsage([]byte("total = 0.00M  used = 0.00M  free = 0.00M\n"))
	if total != 0 || used != 0 {
		t.Errorf("total, used = %d, %d, want 0, 0", total, used)
	}

	if total, used = parseSwapUsage([]byte("")); total != 0 || used != 0 {
		t.Errorf("empty output gave %d, %d", total, used)
	}
}

func TestParseSizeSuffix(t *testing.T) {
	cases := map[string]uint64{
		"512.00M": 512 * (1 << 20),
		"2.00G":   2 * (1 << 30),
		"1024K":   1024 * (1 << 10),
		"1.00T":   1 << 40,
		"4096":    4096,
		"":        0,
		"abcM":    0,
		"-5.00M":  0,
	}

	for input, want := range cases {
		if got := parseSizeSuffix(input); got != want {
			t.Errorf("parseSizeSuffix(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestParseWindowsLoad(t *testing.T) {
	raw := `{"Cores":8,"BusyPercent":17,"TotalKB":16324812,"FreeKB":8218044,"PageTotalMB":2048,"PageUsedMB":512}`

	got, err := parseWindowsLoad([]byte(raw))
	if err != nil {
		t.Fatalf("parseWindowsLoad: %v", err)
	}
	if got.Cores != 8 {
		t.Errorf("Cores = %d, want 8", got.Cores)
	}
	if !got.HasBusy || got.BusyPercent != 17 {
		t.Errorf("BusyPercent = %d (present %v), want 17", got.BusyPercent, got.HasBusy)
	}
	// Windows keeps no run-queue average, so this must stay absent rather
	// than being reported as zero.
	if got.HasLoadAverage {
		t.Error("HasLoadAverage = true; Windows does not keep one")
	}
	if got.MemTotalBytes != 16324812*1024 {
		t.Errorf("MemTotalBytes = %d", got.MemTotalBytes)
	}
	if got.SwapTotalBytes != 2048*(1<<20) {
		t.Errorf("SwapTotalBytes = %d", got.SwapTotalBytes)
	}
}

func TestParseWindowsLoadHandlesQuotedNumbersAndIdleMachines(t *testing.T) {
	// Older PowerShell quotes numeric CIM fields, and an idle machine really
	// does report zero busy.
	raw := `{"Cores":"4","BusyPercent":"0","TotalKB":"8000000","FreeKB":"6000000","PageTotalMB":null,"PageUsedMB":null}`

	got, err := parseWindowsLoad([]byte(raw))
	if err != nil {
		t.Fatalf("parseWindowsLoad: %v", err)
	}
	if got.Cores != 4 || got.BusyPercent != 0 {
		t.Errorf("got %+v", got)
	}
	// Zero is a reading, not a gap.
	if !got.HasBusy {
		t.Error("HasBusy = false for an idle machine that did answer")
	}
	if got.SwapTotalBytes != 0 {
		t.Errorf("SwapTotalBytes = %d, want 0 when no page file is configured", got.SwapTotalBytes)
	}
}

func TestParseWindowsLoadRejectsAnEmptyAnswer(t *testing.T) {
	if _, err := parseWindowsLoad([]byte("")); err == nil {
		t.Error("parseWindowsLoad accepted an empty response")
	}
	if _, err := parseWindowsLoad([]byte("[]")); err == nil {
		t.Error("parseWindowsLoad accepted a response with no rows")
	}
}
