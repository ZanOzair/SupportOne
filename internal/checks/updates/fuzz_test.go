package updates

import "testing"

func FuzzParseWindowsUpdates(f *testing.F) {
	f.Add([]byte(`{"LastSearchSuccessDate":"/Date(1700000000000)/"}`))
	f.Add([]byte(`{"LastSearchSuccessDate":`))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseWindowsUpdates(data)
		_ = parseMacSoftwareUpdateDate(data)
	})
}

func FuzzParseWindowsUpdateTime(f *testing.F) {
	f.Add("/Date(1700000000000)/")
	f.Add("/Date(")
	f.Add("/Date(99999999999999999999)/")

	f.Fuzz(func(t *testing.T, s string) {
		_ = parseWindowsUpdateTime(s)
	})
}
