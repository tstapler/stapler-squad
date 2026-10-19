package wakes

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// DefaultCronZone is the zone of a cron spec that names none. It is never the
// server's local zone, so a wake means the same thing on every host.
const DefaultCronZone = "UTC"

// ErrInvalidCron is wrapped by every ParseCron failure.
var ErrInvalidCron = errors.New("invalid cron spec")

// CronSchedule is a parsed, zone-resolved 5-field cron expression with minute
// granularity. robfig/cron is used only to compute next times; the dispatcher
// owns the timer (ADR-006 rule 3).
type CronSchedule struct {
	sched cron.Schedule
	wall  cron.Schedule // the same expression read as UTC wall-clock, to find skipped local times
	zone  *time.Location
}

// ParseCron parses expr, an optional "CRON_TZ=<IANA zone> " (or "TZ=") prefix
// followed by a standard 5-field expression. A missing zone means UTC and an
// unknown zone is an error. "@every" is rejected: it is not minute-aligned and
// would bypass the minimum-interval limit.
func ParseCron(expr string) (CronSchedule, error) {
	fields := strings.Fields(expr)
	if len(fields) == 0 {
		return CronSchedule{}, fmt.Errorf("%w: empty expression", ErrInvalidCron)
	}
	zoneName := DefaultCronZone
	rest := fields
	if name, ok := cronZonePrefix(fields[0]); ok {
		zoneName, rest = name, fields[1:]
	}
	if len(rest) == 0 || strings.HasPrefix(rest[0], "@every") {
		return CronSchedule{}, fmt.Errorf("%w: %q", ErrInvalidCron, expr)
	}
	zone, err := time.LoadLocation(zoneName)
	if err != nil {
		return CronSchedule{}, fmt.Errorf("%w: unknown zone %q: %v", ErrInvalidCron, zoneName, err)
	}
	// ParseStandard reads the prefix itself, so hand it a normalized one.
	sched, err := cron.ParseStandard("CRON_TZ=" + zoneName + " " + strings.Join(rest, " "))
	if err != nil {
		return CronSchedule{}, fmt.Errorf("%w: %v", ErrInvalidCron, err)
	}
	wall, err := cron.ParseStandard("CRON_TZ=UTC " + strings.Join(rest, " "))
	if err != nil {
		return CronSchedule{}, fmt.Errorf("%w: %v", ErrInvalidCron, err)
	}
	return CronSchedule{sched: sched, wall: wall, zone: zone}, nil
}

func cronZonePrefix(field string) (string, bool) {
	for _, p := range []string{"CRON_TZ=", "TZ="} {
		if name, ok := strings.CutPrefix(field, p); ok {
			return name, true
		}
	}
	return "", false
}

// Zone is the IANA zone name the schedule runs in (shown on the Scheduled prompts row).
func (c CronSchedule) Zone() string { return c.zone.String() }

// Next returns the first fire time strictly after t, in UTC. A wall-clock time
// that does not exist (spring-forward gap) is skipped, as robfig documents.
func (c CronSchedule) Next(after time.Time) time.Time {
	return c.sched.Next(after).UTC()
}

// NextAfterFire returns the next fire time once the occurrence scheduled for
// `scheduled` has been delivered at `now`. It searches after the later of the
// two, so a clock that jumped backward never re-fires an occurrence already
// delivered, and an ambiguous fall-back wall-clock time fires only at its
// first instant.
func (c CronSchedule) NextAfterFire(scheduled, now time.Time) time.Time {
	after := now
	if scheduled.After(after) {
		after = scheduled
	}
	next := c.Next(after)
	if c.sameWallClockMinute(next, scheduled) {
		next = c.Next(next)
	}
	return next
}

func (c CronSchedule) sameWallClockMinute(a, b time.Time) bool {
	const layout = "2006-01-02 15:04"
	return a.In(c.zone).Format(layout) == b.In(c.zone).Format(layout)
}

// maxGapScan bounds SkippedGap's walk; a gap is at most one hour wide.
const maxGapScan = 10_000

// SkippedGap reports the first wall-clock occurrence strictly between after and
// next that does not exist in the zone (a spring-forward gap), which Next
// silently skipped. The dispatcher logs it as reason=dst_gap.
func (c CronSchedule) SkippedGap(after, next time.Time) (time.Time, bool) {
	limit := wallClockUTC(next.In(c.zone))
	t := c.wall.Next(wallClockUTC(after.In(c.zone)))
	for range maxGapScan {
		if !t.Before(limit) {
			return time.Time{}, false
		}
		local := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, c.zone)
		if local.Hour() != t.Hour() || local.Minute() != t.Minute() {
			return t, true
		}
		t = c.wall.Next(t)
	}
	return time.Time{}, false
}

// wallClockUTC re-labels t's local wall-clock fields as UTC.
func wallClockUTC(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.UTC)
}
