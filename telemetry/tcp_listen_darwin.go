//go:build darwin

package telemetry

import "golang.org/x/sys/unix"

func readListenCounters() (map[string]int64, bool) { return nil, false }

func readSomaxconn() (int64, bool) {
	v, err := unix.SysctlUint32("kern.ipc.somaxconn")
	return int64(v), err == nil
}
