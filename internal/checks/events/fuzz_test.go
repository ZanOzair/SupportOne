package events

import "testing"

// Log lines are the closest thing here to hostile input: they contain whatever
// any program on the machine chose to write, including bytes nobody intended.

func FuzzParseWindowsEvents(f *testing.F) {
	f.Add([]byte(`[{"Id":41,"ProviderName":"Kernel-Power","TimeCreated":"/Date(1700000000000)/"}]`))
	f.Add([]byte(`[{"Id":`))
	f.Add([]byte(`{}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseWindowsEvents(data)
	})
}

func FuzzParseJournal(f *testing.F) {
	f.Add([]byte(`{"PRIORITY":"3","MESSAGE":"disk error","__REALTIME_TIMESTAMP":"1700000000000000"}`))
	f.Add([]byte(`{"MESSAGE":`))
	f.Add([]byte("\x00\xff\xfe"))

	f.Fuzz(func(t *testing.T, data []byte) {
		_ = parseJournal(data)
	})
}

func FuzzParseMacLog(f *testing.F) {
	f.Add([]byte("2026-01-01 10:00:00.000000+0000 0x0 Error 0x0 0 0 kernel: something failed\n"))
	f.Add([]byte("2026-01-01"))

	f.Fuzz(func(t *testing.T, data []byte) {
		_ = parseMacLog(data)
	})
}

func FuzzParseMacLogTime(f *testing.F) {
	f.Add("2026-01-01 10:00:00.000000+0000")
	f.Add("not a time")
	f.Add("9999999999-99-99")

	f.Fuzz(func(t *testing.T, s string) {
		_ = parseMacLogTime(s)
	})
}
