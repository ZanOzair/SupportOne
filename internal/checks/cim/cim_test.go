package cim

import (
	"testing"
	"time"
)

type sample struct {
	Name  string `json:"Name"`
	Size  Uint   `json:"Size"`
	Count Int    `json:"Count"`
}

func TestUnmarshalAcceptsObjectAndArray(t *testing.T) {
	one, err := Unmarshal[sample]([]byte(`{"Name":"C:","Size":"511103954944","Count":1}`))
	if err != nil {
		t.Fatalf("Unmarshal object: %v", err)
	}
	if len(one) != 1 || one[0].Size != 511103954944 {
		t.Fatalf("object decode = %+v", one)
	}

	many, err := Unmarshal[sample]([]byte(`[{"Name":"C:","Size":1},{"Name":"D:","Size":2}]`))
	if err != nil {
		t.Fatalf("Unmarshal array: %v", err)
	}
	if len(many) != 2 || many[1].Name != "D:" {
		t.Fatalf("array decode = %+v", many)
	}
}

func TestUnmarshalEmptyIsNotAnError(t *testing.T) {
	for _, in := range []string{"", "   ", "null"} {
		got, err := Unmarshal[sample]([]byte(in))
		if err != nil {
			t.Errorf("Unmarshal(%q): %v", in, err)
		}
		if len(got) != 0 {
			t.Errorf("Unmarshal(%q) = %+v, want none", in, got)
		}
	}
}

func TestNumbersAcceptEveryShapePowerShellEmits(t *testing.T) {
	got, err := Unmarshal[sample]([]byte(`{"Size":8589934592.0,"Count":null}`))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got[0].Size != 8589934592 {
		t.Errorf("Size = %d, want 8589934592", got[0].Size)
	}
	if got[0].Count != 0 {
		t.Errorf("Count = %d, want 0 for null", got[0].Count)
	}

	if _, err := Unmarshal[sample]([]byte(`{"Size":"not a number"}`)); err == nil {
		t.Error("expected an error for an unparseable number")
	}
}

func TestParseTime(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want time.Time
	}{
		{"powershell 5.1", "/Date(1693651200000)/", time.UnixMilli(1693651200000).UTC()},
		{"with offset", "/Date(1693651200000+0800)/", time.UnixMilli(1693651200000).UTC()},
		{"powershell 7", "2026-09-02T13:20:00Z", time.Date(2026, 9, 2, 13, 20, 0, 0, time.UTC)},
		{"empty", "", time.Time{}},
		{"unterminated", "/Date(1693651200000", time.Time{}},
		{"garbage", "yesterday", time.Time{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseTime(tc.in); !got.Equal(tc.want) {
				t.Errorf("ParseTime(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
