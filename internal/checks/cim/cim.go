// Package cim decodes the JSON that Windows PowerShell's ConvertTo-Json
// produces for CIM/WMI queries, and the loosely typed numbers other OS tools
// emit.
//
// Two quirks make a shared decoder worthwhile: a query that matched one
// instance serialises as an object while the same query matching several
// serialises as an array, and numeric fields arrive quoted or unquoted
// depending on the host's PowerShell version.
package cim

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Uint accepts a number or a quoted number.
type Uint uint64

// UnmarshalJSON implements json.Unmarshaler.
func (u *Uint) UnmarshalJSON(data []byte) error {
	s := unquote(data)
	if s == "" {
		*u = 0
		return nil
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		// A float-formatted integer ("8589934592.0") still names a size.
		f, ferr := strconv.ParseFloat(s, 64)
		if ferr != nil || f < 0 {
			return fmt.Errorf("cim: parse unsigned field %q: %w", s, err)
		}
		v = uint64(f)
	}
	*u = Uint(v)
	return nil
}

// Int accepts a number or a quoted number.
type Int int

// UnmarshalJSON implements json.Unmarshaler.
func (i *Int) UnmarshalJSON(data []byte) error {
	s := unquote(data)
	if s == "" {
		*i = 0
		return nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		f, ferr := strconv.ParseFloat(s, 64)
		if ferr != nil {
			return fmt.Errorf("cim: parse integer field %q: %w", s, err)
		}
		v = int(f)
	}
	*i = Int(v)
	return nil
}

func unquote(data []byte) string {
	s := strings.Trim(strings.TrimSpace(string(data)), `"`)
	if s == "null" {
		return ""
	}
	return s
}

// Unmarshal decodes ConvertTo-Json output that is one object when the query
// matched a single instance and an array when it matched several.
func Unmarshal[T any](data []byte) ([]T, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}

	if strings.HasPrefix(trimmed, "[") {
		var many []T
		if err := json.Unmarshal([]byte(trimmed), &many); err != nil {
			return nil, fmt.Errorf("cim: parse array: %w", err)
		}
		return many, nil
	}

	var one T
	if err := json.Unmarshal([]byte(trimmed), &one); err != nil {
		return nil, fmt.Errorf("cim: parse object: %w", err)
	}
	return []T{one}, nil
}

// ParseTime reads the two shapes PowerShell emits for a DateTime: the
// "/Date(milliseconds)/" form from Windows PowerShell 5.1 and the ISO-8601 form
// from PowerShell 7. An unparseable value yields the zero time, which reports
// render as "not reported" rather than as a guess.
func ParseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}

	if rest, ok := strings.CutPrefix(s, "/Date("); ok {
		digits, ok := strings.CutSuffix(rest, ")/")
		if !ok {
			return time.Time{}
		}
		// The value may carry a timezone offset: /Date(1693651200000+0800)/.
		if i := strings.IndexAny(digits, "+-"); i > 0 {
			digits = digits[:i]
		}
		if ms, err := strconv.ParseInt(digits, 10, 64); err == nil {
			return time.UnixMilli(ms).UTC()
		}
		return time.Time{}
	}

	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05.9999999", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
