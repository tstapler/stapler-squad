//go:build linux

package telemetry

import (
	"os"
	"strconv"
	"strings"
)

func readListenCounters() (map[string]int64, bool) {
	f, err := os.Open("/proc/net/netstat")
	if err != nil {
		return nil, false
	}
	defer f.Close()
	c := parseTcpExt(f, "ListenOverflows", "ListenDrops")
	return c, len(c) > 0
}

func readSomaxconn() (int64, bool) {
	b, err := os.ReadFile("/proc/sys/net/core/somaxconn")
	if err != nil {
		return 0, false
	}
	v, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	return v, err == nil
}
