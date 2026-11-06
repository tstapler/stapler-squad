package deliverygate

import (
	"sort"
	"time"
)

// Persist returns the document to save (writer fields left for the store).
func (s *Stats) Persist() PersistedStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := PersistedStats{
		Version:      StatsFileVersion,
		ProcessStart: s.processStart.UTC(),
		FlagHistory:  append([]FlagChange(nil), s.history...),
		LastOffFlip:  map[string]time.Time{},
	}
	for k, v := range s.lastOffFlip {
		out.LastOffFlip[k] = v.UTC()
	}
	for _, b := range s.sortedLocked() {
		out.Buckets = append(out.Buckets, b.persisted())
	}
	return out
}

func (b *statBucket) persisted() PersistedBucket {
	pb := PersistedBucket{
		HourStart: b.start, UptimeMs: b.uptime.Milliseconds(), GateOnMs: b.gateOn.Milliseconds(),
		Seen: b.seen, WhileOn: b.whileOn, RoutineWhileOn: b.routineWhileOn,
	}
	if len(b.gateOnByKind) > 0 {
		pb.GateOnMsByKind = map[string]int64{}
		for k, d := range b.gateOnByKind {
			pb.GateOnMsByKind[k] = d.Milliseconds()
		}
	}
	for k, n := range b.counts {
		if n != 0 {
			pb.Counters = append(pb.Counters, PersistedCounter{Counter: k.name, Kind: k.kind, Class: k.class, Count: n})
		}
	}
	sort.Slice(pb.Counters, func(i, j int) bool {
		a, c := pb.Counters[i], pb.Counters[j]
		if a.Counter != c.Counter {
			return a.Counter < c.Counter
		}
		if a.Kind != c.Kind {
			return a.Kind < c.Kind
		}
		return a.Class < c.Class
	})
	return pb
}

func (s *Stats) sortedLocked() []*statBucket {
	out := make([]*statBucket, 0, len(s.buckets))
	for _, b := range s.buckets {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].start.Before(out[j].start) })
	if len(out) > BucketCount {
		out = out[len(out)-BucketCount:]
	}
	return out
}

// Merge folds a persisted document into memory per hour_start: counters are
// summed, durations summed and clamped to one hour, flag history merged by
// time and truncated. Load is Merge into an empty Stats; the foreign-writer
// resume uses it with a live one.
func (s *Stats) Merge(p PersistedStats) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.prevStart.IsZero() && !p.ProcessStart.IsZero() {
		s.prevStart = p.ProcessStart
	}
	for _, pb := range p.Buckets {
		b := s.bucketLocked(pb.HourStart)
		b.uptime = clampHour(b.uptime + time.Duration(pb.UptimeMs)*time.Millisecond)
		b.gateOn = clampHour(b.gateOn + time.Duration(pb.GateOnMs)*time.Millisecond)
		for k, ms := range pb.GateOnMsByKind {
			b.gateOnByKind[k] = clampHour(b.gateOnByKind[k] + time.Duration(ms)*time.Millisecond)
		}
		b.seen += pb.Seen
		b.whileOn += pb.WhileOn
		b.routineWhileOn += pb.RoutineWhileOn
		for _, c := range pb.Counters {
			b.counts[statKey{c.Counter, c.Kind, c.Class}] += c.Count
		}
	}
	s.mergeHistoryLocked(p.FlagHistory)
	for scope, at := range p.LastOffFlip {
		if at.After(s.lastOffFlip[scope]) {
			s.lastOffFlip[scope] = at
		}
	}
	s.pruneLocked()
}

func clampHour(d time.Duration) time.Duration {
	if d > time.Hour {
		return time.Hour
	}
	return d
}

func (s *Stats) mergeHistoryLocked(in []FlagChange) {
	seen := map[FlagChange]bool{}
	var all []FlagChange
	for _, c := range append(append([]FlagChange(nil), s.history...), in...) {
		c.At = c.At.UTC()
		if !seen[c] {
			seen[c] = true
			all = append(all, c)
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].At.Before(all[j].At) })
	if len(all) > FlagHistoryLimit {
		all = all[len(all)-FlagHistoryLimit:]
	}
	s.history = all
}
