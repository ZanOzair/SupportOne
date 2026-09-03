package drivers

import "testing"

func TestParseProblemDevices(t *testing.T) {
	fixture := []byte(`[{"Name":"Realtek Audio","DeviceID":"HDAUDIO\\FUNC_01","ConfigManagerErrorCode":10},` +
		`{"Name":"USB Dock","DeviceID":"USB\\VID_17E9","ConfigManagerErrorCode":45},` +
		`{"Name":"Healthy NIC","DeviceID":"PCI\\VEN_8086","ConfigManagerErrorCode":0},` +
		`{"Name":"Mystery Device","DeviceID":"PCI\\VEN_0000","ConfigManagerErrorCode":99}]`)

	devices, err := parseProblemDevices(fixture)
	if err != nil {
		t.Fatalf("parseProblemDevices: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("got %d devices, want 2 (an unplugged dock is not a driver fault): %+v", len(devices), devices)
	}
	if devices[0].Name != "Realtek Audio" || devices[0].Meaning != "cannot start" {
		t.Errorf("device = %+v", devices[0])
	}
	if devices[1].Meaning != "" {
		t.Errorf("unrecognised code %d was described as %q; it should be reported by number only",
			devices[1].ErrorCode, devices[1].Meaning)
	}
}

func TestParseProblemDevicesWithSingleEntry(t *testing.T) {
	devices, err := parseProblemDevices([]byte(`{"Name":"Printer","DeviceID":"USB\\P","ConfigManagerErrorCode":28}`))
	if err != nil {
		t.Fatalf("parseProblemDevices: %v", err)
	}
	if len(devices) != 1 || devices[0].Meaning != "the drivers are not installed" {
		t.Errorf("devices = %+v", devices)
	}
}

func TestParseProblemDevicesWithNothingWrong(t *testing.T) {
	devices, err := parseProblemDevices([]byte(`null`))
	if err != nil {
		t.Fatalf("parseProblemDevices: %v", err)
	}
	if len(devices) != 0 {
		t.Errorf("devices = %+v, want none", devices)
	}
}
