package deliverygate

import (
	"strings"
	"time"
)

// BucketSnapshot is one hour as the RPC reports it.
type BucketSnapshot struct {
	HourStart      time.Time
	UptimeSeconds  int32
	GateOnSeconds  int32
	GateOnByKind   map[string]int32
	Seen           int64
	WhileOn        int64
	RoutineWhileOn int64
	Counters       []PersistedCounter
}

// Soak is the server-computed evidence the Stage 3 prerequisites read.
type Soak struct {
	GateOnHours                float64
	RoutineEventsWhileOn       int64
	LastOffFlipAt              time.Time
	FailureDeliveredWhileOn    int64
	NeedsHumanDeliveredWhileOn int64
	ProbeDeliveredWhileOn      int64
	SoakStreakHours            float64
	SoakStreakHoursByKind      map[string]float64
}

// StatsSnapshot is the in-memory view behind GetDeliveryGateStats.
type StatsSnapshot struct {
	ProcessStart         time.Time
	PreviousProcessStart time.Time
	Uptime               time.Duration
	Buckets              []BucketSnapshot
	FlagHistory          []FlagChange
	EventsByKind24h      map[string]int64
	Soak                 Soak
}

// Snapshot credits the time up to now and returns the derived view.
func (s *Stats) Snapshot(now time.Time) StatsSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creditLocked(now)
	snap := StatsSnapshot{
		ProcessStart: s.processStart.UTC(), PreviousProcessStart: s.prevStart,
		Uptime:          now.Sub(s.processStart),
		FlagHistory:     append([]FlagChange(nil), s.history...),
		EventsByKind24h: map[string]int64{},
	}
	buckets := s.sortedLocked()
	cutoff := hourOf(now).Add(-23 * time.Hour)
	for _, b := range buckets {
		snap.Buckets = append(snap.Buckets, b.snapshot())
		if !b.start.Before(cutoff) {
			for k, n := range b.counts {
				if k.name == StatSuppressed || k.name == StatWouldSuppress || k.name == StatHiddenDelivered {
					snap.EventsByKind24h[k.kind] += n
				}
			}
		}
	}
	snap.Soak = s.soakLocked(buckets)
	return snap
}

func (b *statBucket) snapshot() BucketSnapshot {
	bs := BucketSnapshot{
		HourStart: b.start, UptimeSeconds: int32(b.uptime / time.Second), GateOnSeconds: int32(b.gateOn / time.Second),
		GateOnByKind: map[string]int32{}, Seen: b.seen, WhileOn: b.whileOn, RoutineWhileOn: b.routineWhileOn,
		Counters: b.persisted().Counters,
	}
	for k, d := range b.gateOnByKind {
		bs.GateOnByKind[k] = int32(d / time.Second)
	}
	return bs
}

func (s *Stats) soakLocked(buckets []*statBucket) Soak {
	lastOff := s.lastOffFlip["global"]
	soak := Soak{LastOffFlipAt: lastOff, SoakStreakHoursByKind: map[string]float64{}}
	after := func(b *statBucket, flip time.Time) bool {
		return flip.IsZero() || b.start.After(hourOf(flip))
	}
	for _, b := range buckets {
		if b.routineWhileOn > 0 && after(b, lastOff) {
			soak.SoakStreakHours += minDur(b.gateOn, b.uptime).Hours()
		}
		if after(b, lastOff) {
			soak.GateOnHours += minDur(b.gateOn, b.uptime).Hours()
			soak.RoutineEventsWhileOn += b.routineWhileOn
			for k, n := range b.counts {
				switch k.name {
				case StatDeliveredWhileOn:
					if k.class == ClassFailure.String() {
						soak.FailureDeliveredWhileOn += n
					} else if k.class == ClassNeedsHuman.String() {
						soak.NeedsHumanDeliveredWhileOn += n
					}
				case StatProbeDeliveredWhileOn:
					soak.ProbeDeliveredWhileOn += n
				}
			}
		}
	}
	for _, k := range statKinds {
		kindOff := s.lastOffFlip["kind:"+string(k)]
		var hours float64
		for _, b := range buckets {
			if !after(b, kindOff) {
				continue
			}
			var routine int64
			for key, n := range b.counts {
				if key.name == StatSuppressed && key.kind == string(k) {
					routine += n
				}
			}
			if routine > 0 {
				hours += minDur(b.gateOnByKind[string(k)], b.uptime).Hours()
			}
		}
		soak.SoakStreakHoursByKind[string(k)] = hours
	}
	return soak
}

func minDur(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// KindFromScope returns the kind of a "kind:<name>" scope.
func KindFromScope(scope string) (HiddenKind, bool) {
	k, ok := strings.CutPrefix(scope, "kind:")
	return HiddenKind(k), ok
}
