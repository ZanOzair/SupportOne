package storage

import (
	"bufio"
	"bytes"
	"strings"
)

// parseDiskutilInfo reads the sections `diskutil info -all` prints, one per
// device, and keeps the whole-disk entries with a SMART verdict.
//
// macOS reports "Verified" for a healthy drive, "Failing" for one that is not,
// and "Not Supported" for most external enclosures — which is recorded as
// unknown, not as healthy.
func parseDiskutilInfo(data []byte) []disk {
	var (
		out     []disk
		current disk
		hasInfo bool
	)

	flush := func() {
		if hasInfo && current.Name != "" {
			out = append(out, current)
		}
		current = disk{Status: statusUnknown}
		hasInfo = false
	}
	current = disk{Status: statusUnknown}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "****") {
			flush()
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		switch key {
		case "Device Identifier":
			current.Name = value
		case "Device / Media Name":
			current.Model = value
		case "Whole":
			hasInfo = hasInfo || strings.EqualFold(value, "Yes")
		case "SMART Status":
			hasInfo = true
			current.Status = macSMARTStatus(value)
		}
	}
	flush()
	return out
}

func macSMARTStatus(value string) status {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "verified":
		return statusHealthy
	case "failing":
		return statusFailing
	default:
		return statusUnknown
	}
}
