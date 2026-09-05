package security

import "testing"

// A security check that crashes reports nothing, and reporting nothing about
// disk encryption is worse than reporting that it could not tell.

func FuzzParseUnixSecurity(f *testing.F) {
	f.Add([]byte("FileVault is On."))
	f.Add([]byte("Firewall is enabled. (State = 1)"))
	f.Add([]byte("TYPE=\"crypto_LUKS\"\n"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, data []byte) {
		_ = parseFDESetup(data)
		_ = parseALFGlobalState(data)
		_, _ = parseLUKS(data)
	})
}

func FuzzParseWindowsSecurity(f *testing.F) {
	f.Add([]byte(`[{"Name":"Domain","Enabled":true}]`))
	f.Add([]byte(`{"AntivirusEnabled":true,"displayName":"Defender"}`))
	f.Add([]byte(`[{"ProtectionStatus":1}]`))
	f.Add([]byte(`{`))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseWindowsFirewall(data)
		_, _, _ = parseWindowsAntivirus(data)
		_, _ = parseWindowsBitLocker(data)
	})
}
