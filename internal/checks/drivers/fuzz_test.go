package drivers

import "testing"

func FuzzParseProblemDevices(f *testing.F) {
	f.Add([]byte(`[{"Name":"Unknown Device","ConfigManagerErrorCode":28}]`))
	f.Add([]byte(`[{"ConfigManagerErrorCode":-1}]`))
	f.Add([]byte(`{`))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseProblemDevices(data)
	})
}
