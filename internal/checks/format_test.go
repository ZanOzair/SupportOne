package checks

import (
	"testing"
	"time"
)

func TestHumanBytes(t *testing.T) {
	tests := []struct {
		bytes uint64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KiB"},
		{8 << 30, "8.0 GiB"},
		{16106127360, "15.0 GiB"},
		{2 << 40, "2.0 TiB"},
	}
	for _, tc := range tests {
		if got := HumanBytes(tc.bytes); got != tc.want {
			t.Errorf("HumanBytes(%d) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "less than a minute"},
		{5 * time.Minute, "5m"},
		{90 * time.Minute, "1h 30m"},
		{50 * time.Hour, "2d 2h"},
	}
	for _, tc := range tests {
		if got := HumanDuration(tc.d); got != tc.want {
			t.Errorf("HumanDuration(%s) = %q, want %q", tc.d, got, tc.want)
		}
	}
}
