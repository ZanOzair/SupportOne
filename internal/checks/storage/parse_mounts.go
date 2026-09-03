package storage

import (
	"bufio"
	"bytes"
	"strings"
)

// mountEntry is one line of /proc/self/mounts.
type mountEntry struct {
	Device string
	Mount  string
	FSType string
}

// pseudoFilesystems are kernel bookkeeping mounts, not drives a person has. A
// report listing them as "disks" would bury the two the user actually cares
// about.
var pseudoFilesystems = map[string]bool{
	"autofs": true, "bpf": true, "cgroup": true, "cgroup2": true,
	"configfs": true, "debugfs": true, "devpts": true, "devtmpfs": true,
	"efivarfs": true, "fuse.gvfsd-fuse": true, "fuse.portal": true,
	"fusectl": true, "hugetlbfs": true, "mqueue": true, "nsfs": true,
	"overlay": true, "proc": true, "pstore": true, "ramfs": true,
	"securityfs": true, "selinuxfs": true, "squashfs": true, "sysfs": true,
	"tmpfs": true, "tracefs": true,
}

// parseMounts reads the device, mount point and filesystem type from the
// mount table, unescaping the octal sequences the kernel uses for spaces.
func parseMounts(data []byte) []mountEntry {
	var out []mountEntry
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		out = append(out, mountEntry{
			Device: unescapeMount(fields[0]),
			Mount:  unescapeMount(fields[1]),
			FSType: fields[2],
		})
	}
	return out
}

// unescapeMount decodes the \040 style escapes the kernel writes for spaces,
// tabs, newlines and backslashes in mount paths.
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	replacer := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return replacer.Replace(s)
}

// isRealFilesystem reports whether a mount is a drive worth showing the user.
func isRealFilesystem(e mountEntry) bool {
	if pseudoFilesystems[e.FSType] {
		return false
	}
	// A real filesystem is backed by a device node or a network share, not by
	// a bare name like "none" or "systemd-1".
	return strings.HasPrefix(e.Device, "/") || strings.Contains(e.Device, ":") || strings.HasPrefix(e.Device, `\\`)
}

// blockDeviceIsPhysical filters out the loop, ram and optical devices that
// /sys/block lists alongside real drives.
func blockDeviceIsPhysical(name string) bool {
	for _, prefix := range []string{"loop", "ram", "zram", "dm-", "sr", "md"} {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	return true
}
