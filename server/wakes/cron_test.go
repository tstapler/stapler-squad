package wakes

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func utc(y int, m time.Month, d, h, min int) time.Time {
	return time.Date(y, m, d, h, min, 0, 0, time.UTC)
}

func mustParseCron(t *testing.T, expr string) CronSchedule {
	t.Helper()
	c, err := ParseCron(expr)
	require.NoError(t, err)
	return c
}

func TestCronSchedule_Next_should_ReturnNextBoundary_When_EveryFiveMinutes(t *testing.T) {
	t.Parallel()
	c := mustParseCron(t, "*/5 * * * *")
	first := c.Next(utc(2026, 10, 18, 10, 2))
	assert.Equal(t, utc(2026, 10, 18, 10, 5), first)
	assert.Equal(t, utc(2026, 10, 18, 10, 10), c.NextAfterFire(first, first))
}

func TestParseCron_should_DefaultToUTC_When_NoZone(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "UTC", mustParseCron(t, "30 9 * * *").Zone())
	assert.Equal(t, "America/Chicago", mustParseCron(t, "CRON_TZ=America/Chicago 30 9 * * *").Zone())
	assert.Equal(t, "Europe/Paris", mustParseCron(t, "TZ=Europe/Paris 30 9 * * *").Zone())
	assert.Equal(t, utc(2026, 10, 18, 9, 30), mustParseCron(t, "30 9 * * *").Next(utc(2026, 10, 18, 0, 0)))
}

func TestParseCron_should_Reject_When_Invalid(t *testing.T) {
	t.Parallel()
	for name, expr := range map[string]string{
		"empty":        "  ",
		"unknown zone": "CRON_TZ=Mars/Olympus 0 9 * * *",
		"zone only":    "CRON_TZ=UTC",
		"garbage":      "not a cron",
		"six fields":   "0 0 9 * * *",
		"@every":       "@every 1m",
	} {
		_, err := ParseCron(expr)
		assert.ErrorIs(t, err, ErrInvalidCron, name)
	}
}

// Spring forward: 02:30 America/Chicago does not exist on 2026-03-08.
func TestCronSchedule_Next_should_SkipNonexistentTime_When_SpringForwardGap(t *testing.T) {
	t.Parallel()
	c := mustParseCron(t, "CRON_TZ=America/Chicago 30 2 * * *")
	chicago, err := time.LoadLocation("America/Chicago")
	require.NoError(t, err)

	fired := time.Date(2026, 3, 7, 2, 30, 0, 0, chicago)
	require.Equal(t, fired.UTC(), c.Next(fired.Add(-time.Hour)))

	next := c.NextAfterFire(fired.UTC(), fired.UTC())
	want := time.Date(2026, 3, 9, 2, 30, 0, 0, chicago).UTC()
	assert.Equal(t, want, next, "no fire on 2026-03-08; next is 03-09 02:30 local")
}

// Fall back: 01:30 America/Chicago happens twice on 2026-11-01.
func TestCronSchedule_NextAfterFire_should_FireOnce_When_FallBackRepeatsHour(t *testing.T) {
	t.Parallel()
	c := mustParseCron(t, "CRON_TZ=America/Chicago 30 1 * * *")
	chicago, err := time.LoadLocation("America/Chicago")
	require.NoError(t, err)

	prev := time.Date(2026, 10, 31, 1, 30, 0, 0, chicago).UTC()
	first := c.NextAfterFire(prev, prev)
	require.Equal(t, time.Date(2026, 11, 1, 1, 30, 0, 0, chicago).UTC(), first)
	require.Equal(t, utc(2026, 11, 1, 6, 30), first, "first (CDT) occurrence")

	// Delivered promptly: the repeated 01:30 CST (07:30Z) must not fire again.
	next := c.NextAfterFire(first, first)
	assert.Equal(t, time.Date(2026, 11, 2, 1, 30, 0, 0, chicago).UTC(), next)
}

func TestCronSchedule_NextAfterFire_should_NotRefire_When_ClockJumpsBackward(t *testing.T) {
	t.Parallel()
	c := mustParseCron(t, "*/5 * * * *")
	scheduled := utc(2026, 10, 18, 10, 5)
	jumpedBack := utc(2026, 10, 18, 9, 5)
	assert.Equal(t, utc(2026, 10, 18, 10, 10), c.NextAfterFire(scheduled, jumpedBack))
}

func TestCronSchedule_NextAfterFire_should_SkipMissedOccurrences_When_ClockJumpsForward(t *testing.T) {
	t.Parallel()
	c := mustParseCron(t, "*/5 * * * *")
	scheduled := utc(2026, 10, 18, 10, 5)
	assert.Equal(t, utc(2026, 10, 18, 12, 35), c.NextAfterFire(scheduled, utc(2026, 10, 18, 12, 31)))
}
