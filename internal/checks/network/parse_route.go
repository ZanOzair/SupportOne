package network

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// parseProcRoute reads the default gateway from /proc/net/route, whose address
// columns are little-endian hexadecimal.
func parseProcRoute(data []byte) string {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for lineNo := 0; scanner.Scan(); lineNo++ {
		if lineNo == 0 {
			continue // header
		}
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		// A destination of 00000000 is the default route.
		if fields[1] != "00000000" {
			continue
		}
		if gw, err := parseHexIPv4(fields[2]); err == nil && !gw.IsUnspecified() {
			return gw.String()
		}
	}
	return ""
}

// parseHexIPv4 decodes the little-endian hex form the kernel uses in
// /proc/net/route.
func parseHexIPv4(s string) (net.IP, error) {
	value, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return nil, fmt.Errorf("network: parse route address %q: %w", s, err)
	}
	ip := make(net.IP, 4)
	binary.LittleEndian.PutUint32(ip, uint32(value))
	return ip, nil
}

// parseResolvConf reads the nameserver lines of resolv.conf.
func parseResolvConf(data []byte) []string {
	var out []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "nameserver" {
			continue
		}
		if net.ParseIP(fields[1]) != nil {
			out = append(out, fields[1])
		}
	}
	return out
}
