package wakes

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// stormRig is a dispatcher over `owners` stopped allow_revive owners, all
// waiting on item X, with WIP cap `slots`.
type stormRig struct {
	*dispatcherRig
	adm     *slotAdmission
	targets map[string]*spyTarget
	rows    map[string]*session.WakeTriggerData
}

func newStormRig(t *testing.T, slots int, stopped, live int) *stormRig {
	t.Helper()
	adm := &slotAdmission{cap: slots}
	r := newDispatcherRig(t, func(c *Config) { c.Admission = adm })
	s := &stormRig{dispatcherRig: r, adm: adm, targets: map[string]*spyTarget{}, rows: map[string]*session.WakeTriggerData{}}
	for i := range stopped {
		name := fmt.Sprintf("stopped-%02d", i)
		s.targets[name] = stoppedOwner(r.owners, adm, name)
		r.steerer.setReady(name, true)
		s.rows[name] = backlogRow(t, r, itemX, "verdict_recorded", rowOpts{owner: name, allowRevive: true})
	}
	for i := range live {
		name := fmt.Sprintf("live-%02d", i)
		r.owners.set(name, OwnerFacts{Exists: true, Live: true})
		r.steerer.setReady(name, true)
		s.rows[name] = backlogRow(t, r, itemX, "verdict_recorded", rowOpts{owner: name})
	}
	r.start(r.d)
	return s
}

func (s *stormRig) revived() (n int) {
	for _, tg := range s.targets {
		if _, started := tg.calls(); started > 0 {
			n++
		}
	}
	return n
}

func (s *stormRig) count(status domain.WakeStatus) int {
	rows, err := s.repo.ListWakeTriggersByStatus(context.Background(), status)
	require.NoError(s.t, err)
	return len(rows)
}

// settled waits for a revive wave; 50 serialized admissions are slow under -race.
func (s *stormRig) settled(cond func() bool) {
	s.t.Helper()
	require.Eventually(s.t, cond, 60*time.Second, 10*time.Millisecond)
}

// The storm is stated as revives of stopped owners, not spawns: wakes target
// existing sessions (Story 4.8).
func TestStorm_should_reviveAtMost2PerCycleAndLoseNone_When_50AllowReviveTriggersOnItemX(t *testing.T) {
	t.Parallel()
	const owners, slots = 50, 2
	s := newStormRig(t, slots, owners, 0)

	s.bus.Publish(verdictEvent(itemX))
	s.settled(func() bool {
		return s.count(domain.WakeStatusDelivered)+s.count(domain.WakeStatusNeedsRevive) == owners
	})
	s.d.WaitIdle()

	assert.Equal(t, slots, s.revived(), "only the free slots are revived in the first cycle")
	assert.Equal(t, slots, s.count(domain.WakeStatusDelivered))
	assert.Equal(t, owners-slots, s.count(domain.WakeStatusNeedsRevive), "the rest wait, none lost")

	for cycle := 1; s.count(domain.WakeStatusDelivered) < owners; cycle++ {
		require.Less(t, cycle, owners, "the storm must drain")
		before := s.revived()
		s.adm.live.Store(0) // the revived sessions finished and freed their slots
		s.d.Reconcile(context.Background())
		s.d.WaitIdle()
		assert.LessOrEqual(t, s.revived()-before, slots, "cycle %d", cycle)
	}

	assert.Equal(t, owners, s.count(domain.WakeStatusDelivered), "every wake is eventually delivered")
	assert.Len(t, s.steerer.pane.written(), owners)
	for name, tg := range s.targets {
		recovered, started := tg.calls()
		assert.Equal(t, 1, started, name)
		assert.Equal(t, 1, recovered, name)
	}
}

func TestStorm_should_reviveExactlyOneOwner_When_ThreeStoppedOwnersRaceForOneSlot(t *testing.T) {
	t.Parallel()
	s := newStormRig(t, 1, 3, 0)

	s.bus.Publish(verdictEvent(itemX))
	s.settled(func() bool { return s.count(domain.WakeStatusDelivered)+s.count(domain.WakeStatusNeedsRevive) == 3 })
	s.d.WaitIdle()

	assert.Equal(t, 1, s.revived(), "the revive lock spans Admit until the revived session is live")
	assert.Equal(t, 1, s.count(domain.WakeStatusDelivered))
	assert.Equal(t, 2, s.count(domain.WakeStatusNeedsRevive))
}

func TestStorm_should_wakeLiveOwnersWithoutRevive_When_mixedLiveAndStopped(t *testing.T) {
	t.Parallel()
	const stopped, live, slots = 10, 10, 2
	s := newStormRig(t, slots, stopped, live)

	s.bus.Publish(verdictEvent(itemX))
	s.settled(func() bool {
		return s.count(domain.WakeStatusDelivered)+s.count(domain.WakeStatusNeedsRevive) == stopped+live
	})
	s.d.WaitIdle()

	for name, row := range s.rows {
		if name[:4] == "live" {
			assert.Equal(t, domain.WakeStatusDelivered, s.status(row.ID), name)
		}
	}
	assert.Equal(t, live+slots, s.count(domain.WakeStatusDelivered))
	assert.Equal(t, slots, s.revived(), "only stopped owners are ever revived")
	assert.Equal(t, stopped-slots, s.count(domain.WakeStatusNeedsRevive))
}

// recordingSteerer logs every confirmed delivery with its simulated time.
type recordingSteerer struct {
	Steerer
	clock *FakeClock
	mu    sync.Mutex
	log   map[string][]time.Time
}

func (s *recordingSteerer) WriteWake(ctx context.Context, owner, text string) (WriteOutcome, error) {
	out, err := s.Steerer.WriteWake(ctx, owner, text)
	if err == nil && out == WriteDelivered {
		s.mu.Lock()
		s.log[owner] = append(s.log[owner], s.clock.Now())
		s.mu.Unlock()
	}
	return out, err
}

func (s *recordingSteerer) count(owner string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.log[owner])
}

// maxInRollingHour is the largest number of the times that fall in any window of one hour.
func maxInRollingHour(times []time.Time) int {
	most := 0
	for i := range times {
		n := 0
		for _, u := range times {
			if !u.Before(times[i]) && u.Sub(times[i]) < time.Hour {
				n++
			}
		}
		most = max(most, n)
	}
	return most
}

// stormRun drives `owners` agents that each schedule an aggressive cron wake and
// re-schedule from every woken turn, for `hours` simulated hours in 1-minute steps.
type stormResult struct {
	perOwnerMax    int
	globalMax      int
	gaugeMax       int
	maxGeneration  int
	limitHits      int64
	totalDelivered int
}

func runStorm(t *testing.T, owners int, settings LimitSettings, cron string, hours int) stormResult {
	t.Helper()
	ctx := context.Background()
	rec := &recordingSteerer{clock: nil, log: map[string][]time.Time{}}
	r := newDispatcherRig(t, func(c *Config) {
		c.Limits = settings
		rec.Steerer = c.Steerer
		c.Steerer = rec
	})
	rec.clock = r.clock
	names := make([]string, owners)
	for i := range owners {
		names[i] = fmt.Sprintf("agent-%02d", i)
		r.steerer.setReady(names[i], true)
	}
	var turn int // a distinct payload per turn, so creates are not deduplicated
	schedule := func(owner string) {
		turn++
		action, err := domain.NewWakeAction(domain.WakeActionSteerText, fmt.Sprintf("check the build %d", turn))
		require.NoError(t, err)
		gen, err := r.d.NextGeneration(ctx, owner)
		require.NoError(t, err)
		_, _, err = r.d.Schedule(ctx, ScheduleRequest{Owner: owner, Spec: domain.CronSpec{Expr: cron}, Action: action, MaxFires: 48, Generation: gen})
		var capErr *CapExceededError
		if err != nil {
			require.ErrorAs(t, err, &capErr, "the only refusals are typed limit hits")
		}
	}
	for _, o := range names {
		schedule(o)
	}

	seen := map[string]int{}
	var res stormResult
	for range hours * 60 {
		r.clock.Advance(time.Minute)
		r.d.timer.FireDue(ctx)
		for _, o := range names {
			res.gaugeMax = max(res.gaugeMax, r.d.DeliveredPerOwnerHour(o))
			if n := rec.count(o); n > seen[o] {
				seen[o] = n
				schedule(o) // the woken turn re-schedules itself
			}
		}
	}

	var all []time.Time
	for _, o := range names {
		times := rec.log[o]
		all = append(all, times...)
		res.perOwnerMax = max(res.perOwnerMax, maxInRollingHour(times))
		rows, err := r.repo.ListWakeTriggersByOwner(ctx, o)
		require.NoError(t, err)
		for _, row := range rows {
			res.maxGeneration = max(res.maxGeneration, row.Generation)
		}
	}
	res.globalMax = maxInRollingHour(all)
	res.totalDelivered = len(all)
	m := r.d.Metrics()
	for _, limit := range []string{LimitFiresPerOwnerHour, LimitFiresGlobalHour, LimitGeneration, LimitPendingPerOwner} {
		for _, phase := range []string{PhaseFire, PhaseCreate} {
			res.limitHits += m.Value(MetricLimitHit, limit, phase)
		}
	}
	return res
}

// The $106.69 idle-wait shape (research/pitfalls.md section 4): the assertion
// is the numeric ceiling, not the absence of a bug.
func TestStorm_should_neverExceed6DeliveriesPerOwnerHourAnd60Global_When_cronChainReschedulingFor3SimulatedHours(t *testing.T) {
	t.Parallel()
	one := runStorm(t, 1, LimitSettings{}, "*/10 * * * *", 3)
	assert.LessOrEqual(t, one.perOwnerMax, 6, "per-owner rolling hour")
	assert.LessOrEqual(t, one.gaugeMax, 6, "wake_delivered_per_owner_hour")
	assert.LessOrEqual(t, one.maxGeneration, 5, "chain generation ceiling")
	assert.Positive(t, one.limitHits, "wake_limit_hit_total records the refusals")
	assert.Positive(t, one.totalDelivered, "the wakes do run; the limits only bound them")

	t.Logf("1 owner: %+v", one)
	twenty := runStorm(t, 20, LimitSettings{}, "*/10 * * * *", 3)
	t.Logf("20 owners: %+v", twenty)
	assert.LessOrEqual(t, twenty.perOwnerMax, 6)
	assert.LessOrEqual(t, twenty.globalMax, 60, "global rolling hour")
	assert.LessOrEqual(t, twenty.maxGeneration, 5)
	assert.Positive(t, twenty.limitHits)
	assert.Greater(t, twenty.totalDelivered, one.totalDelivered)
}

func TestStorm_should_neverExceed12DeliveriesPerOwnerHour_When_floorLoweredTo5MinAndLimitAtCeiling(t *testing.T) {
	t.Parallel()
	res := runStorm(t, 1, LimitSettings{MinCronIntervalMinutes: 5, FiresPerOwnerHour: 12}, "*/5 * * * *", 3)
	t.Logf("floor 5m, limit 12: %+v", res)
	assert.LessOrEqual(t, res.perOwnerMax, 12)
	assert.LessOrEqual(t, res.gaugeMax, 12)
	assert.LessOrEqual(t, res.maxGeneration, 5)
	assert.Positive(t, res.totalDelivered)
}
