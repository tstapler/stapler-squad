//go:build !linux

package telemetry

// Cgroup v2 memory accounting (memory.current/high/max/events/pressure) is a
// Linux kernel feature with no equivalent on macOS — cgroup_linux.go's
// metrics simply aren't registered on non-Linux platforms.

// CgroupMemoryUsageRatio always reports unavailable on non-Linux platforms —
// see cgroup_linux.go's identically-named function.
func CgroupMemoryUsageRatio() (ratio float64, ok bool) {
	return 0, false
}
