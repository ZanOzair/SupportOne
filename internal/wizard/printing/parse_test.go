package printing

import "testing"

func TestServiceRunning(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{
			"a running service",
			"SERVICE_NAME: spooler\n        TYPE               : 110\n        STATE              : 4  RUNNING\n",
			true,
		},
		{
			"a stopped service",
			"SERVICE_NAME: spooler\n        STATE              : 1  STOPPED\n",
			false,
		},
		{
			// A service that is on its way up cannot print yet, so it is not
			// treated as though it can.
			"a service starting up",
			"SERVICE_NAME: spooler\n        STATE              : 2  START_PENDING\n",
			false,
		},
		{"nothing was returned", "", false},
		{"an error message", "[SC] EnumQueryServicesStatus:OpenService FAILED 1060", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := serviceRunning([]byte(tc.raw)); got != tc.want {
				t.Errorf("serviceRunning = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDefaultPrinterName(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"one printer", `{"Name":"HP LaserJet"}`, "HP LaserJet"},
		{"with whitespace", "  {\"Name\":\"Brother DCP\"}  \n", "Brother DCP"},
		{"an array, as PowerShell emits for more than one", `[{"Name":"First"},{"Name":"Second"}]`, "First"},
		{"no default printer", "", ""},
		{"an explicit null", "null", ""},
		{"an empty array", "[]", ""},
		{"an object with no name", `{"Default":true}`, ""},
		{"output that is not JSON at all", "Get-CimInstance : The term is not recognized", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := defaultPrinterName([]byte(tc.raw)); got != tc.want {
				t.Errorf("defaultPrinterName(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}
