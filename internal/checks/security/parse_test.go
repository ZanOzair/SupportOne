package security

import "testing"

func TestParseFDESetup(t *testing.T) {
	tests := []struct {
		in   string
		want state
	}{
		{"FileVault is On.\n", stateOn},
		{"FileVault is Off.\n", stateOff},
		{"FileVault is Off, Deferred enablement appears to be active for user alex.\n", stateOff},
		{"something unexpected\n", stateUnknown},
	}
	for _, tc := range tests {
		if got := parseFDESetup([]byte(tc.in)); got != tc.want {
			t.Errorf("parseFDESetup(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseALFGlobalState(t *testing.T) {
	tests := []struct {
		in   string
		want state
	}{
		{"0\n", stateOff},
		{"1\n", stateOn},
		{"2\n", stateOn},
		{"\n", stateUnknown},
	}
	for _, tc := range tests {
		if got := parseALFGlobalState([]byte(tc.in)); got != tc.want {
			t.Errorf("parseALFGlobalState(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseLUKS(t *testing.T) {
	encrypted := []byte(`{"blockdevices":[{"name":"nvme0n1","type":"disk","fstype":null,"children":[` +
		`{"name":"nvme0n1p1","type":"part","fstype":"vfat"},` +
		`{"name":"nvme0n1p2","type":"part","fstype":"crypto_LUKS","children":[` +
		`{"name":"root","type":"crypt","fstype":"ext4"}]}]}]}`)
	got, err := parseLUKS(encrypted)
	if err != nil {
		t.Fatalf("parseLUKS: %v", err)
	}
	if got != stateOn {
		t.Errorf("parseLUKS = %q, want on", got)
	}

	plain := []byte(`{"blockdevices":[{"name":"sda","type":"disk","fstype":null,"children":[` +
		`{"name":"sda1","type":"part","fstype":"ext4"}]}]}`)
	if got, err := parseLUKS(plain); err != nil || got != stateOff {
		t.Errorf("parseLUKS = (%q, %v), want (off, nil)", got, err)
	}

	if _, err := parseLUKS([]byte(`not json`)); err == nil {
		t.Error("expected an error for unparseable lsblk output")
	}
}

func TestParseWindowsFirewall(t *testing.T) {
	allOn := []byte(`[{"Name":"Domain","Enabled":true},{"Name":"Private","Enabled":true},{"Name":"Public","Enabled":true}]`)
	if got, err := parseWindowsFirewall(allOn); err != nil || got != stateOn {
		t.Errorf("all profiles on = (%q, %v), want (on, nil)", got, err)
	}

	publicOff := []byte(`[{"Name":"Domain","Enabled":true},{"Name":"Public","Enabled":false}]`)
	if got, _ := parseWindowsFirewall(publicOff); got != stateOff {
		t.Errorf("one profile off = %q, want off — the public profile is the exposed one", got)
	}

	numeric := []byte(`[{"Name":"Domain","Enabled":1},{"Name":"Public","Enabled":0}]`)
	if got, _ := parseWindowsFirewall(numeric); got != stateOff {
		t.Errorf("numeric form = %q, want off", got)
	}

	if got, _ := parseWindowsFirewall([]byte(`null`)); got != stateUnknown {
		t.Errorf("no profiles = %q, want unknown", got)
	}
}

func TestParseWindowsAntivirus(t *testing.T) {
	// 0x1000 set in productState means real-time protection is running.
	running := []byte(`{"displayName":"Windows Defender","productState":397568}`)
	got, name, err := parseWindowsAntivirus(running)
	if err != nil {
		t.Fatalf("parseWindowsAntivirus: %v", err)
	}
	if got != stateOn || name != "Windows Defender" {
		t.Errorf("got (%q, %q), want (on, Windows Defender)", got, name)
	}

	disabled := []byte(`{"displayName":"Third Party AV","productState":262144}`)
	if got, name, _ := parseWindowsAntivirus(disabled); got != stateOff || name != "Third Party AV" {
		t.Errorf("disabled product = (%q, %q), want (off, Third Party AV)", got, name)
	}

	if got, _, _ := parseWindowsAntivirus([]byte(`null`)); got != stateOff {
		t.Errorf("no registered product = %q, want off", got)
	}
}

func TestParseWindowsBitLocker(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want state
	}{
		{"protected", `{"ProtectionStatus":1}`, stateOn},
		{"unprotected", `{"ProtectionStatus":0}`, stateOff},
		{"string form", `{"ProtectionStatus":"On"}`, stateOn},
		{"no rights", `null`, stateUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseWindowsBitLocker([]byte(tc.in))
			if err != nil {
				t.Fatalf("parseWindowsBitLocker: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
