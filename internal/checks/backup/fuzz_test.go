package backup

import "testing"

func FuzzParseMacBackup(f *testing.F) {
	f.Add([]byte("Name          : Backup Drive\nKind          : Local\n"))
	f.Add([]byte("Name          :"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseDestinationInfo(data)
		_ = parseLatestBackup(data)
	})
}

func FuzzParseShadowCopies(f *testing.F) {
	f.Add([]byte(`[{"InstallDate":"/Date(1700000000000)/"}]`))
	f.Add([]byte(`[{"InstallDate":`))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseShadowCopies(data)
	})
}
