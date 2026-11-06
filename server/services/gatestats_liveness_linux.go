//go:build linux

package services

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// procClockTicks is USER_HZ, 100 on every mainstream Linux build.
const procClockTicks = 100

// processStart derives a process start time from /proc/<pid>/stat field 22
// and /proc/stat btime; ok is false anywhere /proc is unavailable.
func processStart(pid int) (time.Time, bool) {
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
