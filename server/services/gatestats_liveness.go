package services

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// PidLiveness reports whether pid is still the process that wrote the stats
// file at writtenAt.
type PidLiveness func(pid int, writtenAt time.Time) bool

// procClockTicks is USER_HZ, 100 on every mainstream Linux build.
const procClockTicks = 100

// ProcessWriterAlive is signal 0 plus, on Linux, a pid-reuse guard: a process
// that started after the file was written cannot be its writer.
func ProcessWriterAlive(pid int, writtenAt time.Time) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err != nil && !errors.Is(err, syscall.EPERM) {
		return false
	}
	if start, ok := linuxProcessStart(pid); ok && start.After(writtenAt.Add(time.Second)) {
		return false
	}
	return true
}

// linuxProcessStart derives a process start time from /proc/<pid>/stat field 22
// and /proc/stat btime; ok is false anywhere /proc is unavailable.
func linuxProcessStart(pid int) (time.Time, bool) {
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return time.Time{}, false
	}
	// comm (field 2) may contain spaces; fields resume after the last ')'.
	i := strings.LastIndexByte(string(stat), ')')
	if i < 0 {
		return time.Time{}, false
	}
	fields := strings.Fields(string(stat)[i+1:])
	if len(fields) < 20 { // field 22 is index 19 after the state field (field 3)
		return time.Time{}, false
	}
	ticks, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	boot, err := os.ReadFile("/proc/stat")
	if err != nil {
		return time.Time{}, false
	}
	for _, line := range strings.Split(string(boot), "\n") {
		if v, ok := strings.CutPrefix(line, "btime "); ok {
			secs, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			if err != nil {
				return time.Time{}, false
			}
			return time.Unix(secs+ticks/procClockTicks, 0), true
		}
	}
	return time.Time{}, false
}
