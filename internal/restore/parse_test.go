package restore

import "testing"

func TestSequenceNumber(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"a compressed object", `{"SequenceNumber":42}`, "42"},
		{"followed by other fields", `{"SequenceNumber":7,"Description":"before"}`, "7"},
		{"with spacing", `{ "SequenceNumber": 118 }`, "118"},
		{"quoted, as PowerShell sometimes emits", `{"SequenceNumber":"9"}`, `"9"`},
		{"nothing was returned", "", ""},
		{"an unrelated object", `{"Description":"before"}`, ""},
		{"truncated output", `{"SequenceNumber":42`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sequenceNumber([]byte(tc.raw)); got != tc.want {
				t.Errorf("sequenceNumber(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestSnapshotName(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			"tmutil's confirmation",
			"Created local snapshot with date: 2026-09-03-134500\n",
			"2026-09-03-134500",
		},
		{
			"with surrounding noise",
			"NOTE: local snapshots are not backups.\nCreated local snapshot with date: 2026-01-02-000000",
			"2026-01-02-000000",
		},
		{"nothing was returned", "", ""},
		{"a message with no date", "Created local snapshot.\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := snapshotName([]byte(tc.raw)); got != tc.want {
				t.Errorf("snapshotName(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}
