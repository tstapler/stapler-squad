//go:build linux

package server

import (
	"os"
	"strconv"
	"strings"
)

func somaxconn() (int64, bool) {
	b, err := os.ReadFile("/proc/sys/net/core/somaxconn")
	if err != nil {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	return n, err == nil
}

func listenQueueCounters() (overflows, drops int64, ok bool) {
	b, err := os.ReadFile("/proc/net/netstat")
	if err != nil {
		return 0, 0, false
	}
	ov, ok1 := parseTcpExtCounter(b, "ListenOverflows")
	dr, ok2 := parseTcpExtCounter(b, "ListenDrops")
	return ov, dr, ok1 && ok2
}

// openFDCount counts entries of the per-process descriptor directory.
func openFDCount() (int64, bool) {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return 0, false
	}
	return int64(len(entries) - 1), true // minus the descriptor ReadDir itself holds
}
