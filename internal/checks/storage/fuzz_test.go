package storage

import "testing"

// Disk output is the most varied of any check: /proc/mounts on a machine with
// container mounts, diskutil on an encrypted volume, smartctl on a drive that
// answered halfway. None of it should be able to stop a snapshot.

func FuzzParseMounts(f *testing.F) {
	f.Add([]byte("/dev/sda1 / ext4 rw,relatime 0 0\n"))
	f.Add([]byte("/dev/sda1"))
	f.Add([]byte("\x00\x00\x00"))

	f.Fuzz(func(t *testing.T, data []byte) {
		_ = parseMounts(data)
	})
}

func FuzzParseDiskutilInfo(f *testing.F) {
	f.Add([]byte("Device Identifier:        disk0\nDevice / Media Name:      APPLE SSD\n"))
	f.Add([]byte("Device Identifier:"))

	f.Fuzz(func(t *testing.T, data []byte) {
		_ = parseDiskutilInfo(data)
	})
}

func FuzzParseSmartctl(f *testing.F) {
	f.Add([]byte(`{"device":{"name":"/dev/sda"},"smart_status":{"passed":true}}`))
	f.Add([]byte(`{"smart_status":`))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseSmartctl(data, "fallback")
	})
}

func FuzzParseWindowsStorage(f *testing.F) {
	f.Add([]byte(`[{"DriveLetter":"C","Size":500000000000,"SizeRemaining":100000000000}]`))
	f.Add([]byte(`[{"Size":-1}]`))
	f.Add([]byte(`{`))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseWindowsVolumes(data)
		_, _ = parseWindowsDisks(data)
	})
}
