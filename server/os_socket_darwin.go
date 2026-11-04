//go:build darwin

package server

import "golang.org/x/sys/unix"

func somaxconn() (int64, bool) {
	n, err := unix.SysctlUint32("kern.ipc.somaxconn")
	return int64(n), err == nil
}

// listenQueueCounters is unavailable on macOS: the counters live in the binary
// net.inet.tcp.stats struct, whose layout is not a stable API. `netstat -s -p tcp`
// ("listen queue overflows") is the manual check.
func listenQueueCounters() (int64, int64, bool) { return 0, 0, false }

// fdProbeCap bounds the scan; macOS has no cheap per-process descriptor list
// (/dev/fd only shows 0-2), so descriptors are probed with fcntl(F_GETFD).
const fdProbeCap = 32768

func openFDCount() (int64, bool) {
	var rl unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &rl); err != nil {
		return 0, false
	}
	top := int(min(rl.Cur, fdProbeCap))
	var n int64
	for fd := 0; fd < top; fd++ {
		if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); err == nil {
			n++
		}
	}
	return n, true
}
