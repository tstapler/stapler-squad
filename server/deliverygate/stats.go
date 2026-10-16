package deliverygate

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Stats keeps the 72 hourly buckets and derives the soak evidence from them
// (Story 2.8, ADV-B3): gate-on time is credited from the flag snapshot in force
// over each interval, so the soak is derivable rather than remembered. It
// performs no I/O; a StatsStore adapter persists Persist() and feeds Merge().
type Stats struct {
	mu           sync.Mutex
	now          Clock
	processStart time.Time
	prevStart    time.Time // process_start of the window reloaded from disk
	lastTick     time.Time
	cur          FlagSettings // the flag snapshot in force since lastTick
	primed       bool         // false until the first swap (the startup load is not a flip)
	buckets      map[int64]*statBucket
	curBucket    *statBucket // bucket of lastTick's hour; the hot path never reads a clock
	history      []FlagChange
	lastOffFlip  map[string]time.Time

	writerRunning atomic.Bool
}

// BucketCount is the number of hourly buckets kept (a 24h soak plus margin).
const BucketCount = 72

// FlagHistoryLimit bounds the persisted flag-change history.
const FlagHistoryLimit = 20

// ProbeTitlePrefix labels the operator's synthetic probe (Stage 2).
const ProbeTitlePrefix = "[synthetic-probe]"

// Persisted counter names: the reduced (counter, kind, class) key set.
const (
	StatSuppressed            = "suppressed"
	StatWouldSuppress         = "would_suppress"
	StatHiddenDelivered       = "hidden_delivered"
	StatUnresolved            = "unresolved"
	StatDeliveredWhileOn      = "delivered_while_on"
	StatProbeDeliveredWhileOn = "probe_delivered_while_on"
)

// PersistedKeyCeiling is the most keys one bucket can hold: counter names x
// kinds (review, triage, diagnose, other, unresolved) x classes.
const PersistedKeyCeiling = 6 * 5 * 3

var statKinds = []HiddenKind{KindReview, KindTriage, KindDiagnose, KindOther}

type statKey struct{ name, kind, class string }

type statBucket struct {
	start          time.Time
	uptime, gateOn time.Duration
	gateOnByKind   map[string]time.Duration
	seen           int64
	whileOn        int64
	routineWhileOn int64
	counts         map[statKey]int64
}

func newStatBucket(start time.Time) *statBucket {
	return &statBucket{start: start, gateOnByKind: map[string]time.Duration{}, counts: map[statKey]int64{}}
}

// NewStats starts the accumulator at process start with the flag off.
func NewStats(now Clock) *Stats {
	t := now()
	s := &Stats{
		now: now, processStart: t, lastTick: t,
		buckets:     map[int64]*statBucket{},
		lastOffFlip: map[string]time.Time{},
	}
	s.curBucket = s.bucketLocked(t)
	return s
}

// SetWriterRunning is called by the stats writer for its lifetime. Enabling the
// gate is refused while it is false (Task 2.8g).
func (s *Stats) SetWriterRunning(v bool) { s.writerRunning.Store(v) }

// WriterRunning reports whether a stats writer goroutine is running.
func (s *Stats) WriterRunning() bool { return s.writerRunning.Load() }

func hourOf(t time.Time) time.Time { return t.UTC().Truncate(time.Hour) }

func (s *Stats) bucketLocked(t time.Time) *statBucket {
	h := hourOf(t)
	k := h.Unix()
	b := s.buckets[k]
	if b == nil {
		b = newStatBucket(h)
		s.buckets[k] = b
	}
	return b
}

func effectiveFor(f FlagSettings, kind HiddenKind) bool {
	if v, ok := f.KindOverrides[kind]; ok {
		return v
	}
	return f.Global
}

// Tick credits the time since the previous tick to the buckets it spans using
// the snapshot in force (a bucket boundary splits the interval).
func (s *Stats) Tick(now time.Time) {
	s.mu.Lock()
	s.creditLocked(now)
	s.mu.Unlock()
}

func (s *Stats) creditLocked(now time.Time) {
	if !now.After(s.lastTick) {
		return
	}
	for from := s.lastTick; from.Before(now); {
		to := hourOf(from).Add(time.Hour)
		if to.After(now) {
			to = now
		}
		d := to.Sub(from)
		b := s.bucketLocked(from)
		b.uptime += d
		if s.cur.Global {
			b.gateOn += d
		}
		for _, k := range statKinds {
			if effectiveFor(s.cur, k) {
				b.gateOnByKind[string(k)] += d
			}
		}
		from = to
	}
	s.lastTick = now
	s.curBucket = s.bucketLocked(now)
	s.pruneLocked()
}

func (s *Stats) pruneLocked() {
	if len(s.buckets) <= BucketCount {
		return
	}
	keys := make([]int64, 0, len(s.buckets))
	for k := range s.buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, k := range keys[:len(keys)-BucketCount] {
		if s.buckets[k] != s.curBucket {
			delete(s.buckets, k)
		}
	}
}

// onSwap is the FlagCache swap hook: credit the interval to the old snapshot,
// then record any change. It runs on every reload, so a hand edit picked up by
// the ticker is recorded like an RPC flip.
// mutations names the operator mutation behind this swap per scope ("global",
// "kind:review"); an entry is recorded as given, even when the effective value
// did not move (a reset of an explicit false to a default-off flag).
func (s *Stats) onSwap(next FlagSettings, mutations map[string]string) (enabledNow bool) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creditLocked(now)
	prev := s.cur
	s.cur = next
	if !s.primed {
		s.primed = true
		return false
	}
	if m, hinted := mutations["global"]; hinted {
		s.recordChangeLocked("global", m == mutationSetEnabled, now, m)
	} else if prev.Global != next.Global {
		s.recordChangeLocked("global", next.Global, now, setMutation(next.Global))
	}
	if prev.Global && !next.Global {
		s.lastOffFlip["global"] = now
	}
	for _, k := range statKinds {
		pv, pok := prev.KindOverrides[k]
		nv, nok := next.KindOverrides[k]
		scope := "kind:" + string(k)
		m, hinted := mutations[scope]
		switch {
		case hinted:
			s.recordChangeLocked(scope, m == mutationSetEnabled, now, m)
		case nok && (!pok || pv != nv):
			s.recordChangeLocked("kind:"+string(k), nv, now, setMutation(nv))
		case pok && !nok:
			s.recordChangeLocked("kind:"+string(k), false, now, "CLEAR_SCOPE")
		}
		if effectiveFor(prev, k) && !effectiveFor(next, k) {
			s.lastOffFlip["kind:"+string(k)] = now
		}
	}
	return !prev.Global && next.Global
}

// FlagMutation names as the proto spells them after the FLAG_MUTATION_ prefix.
const (
	mutationSetEnabled  = "SET_ENABLED"
	mutationSetDisabled = "SET_DISABLED"
)

func setMutation(v bool) string {
	if v {
		return mutationSetEnabled
	}
	return mutationSetDisabled
}

func (s *Stats) recordChangeLocked(scope string, v bool, at time.Time, mutation string) {
	s.history = append(s.history, FlagChange{Scope: scope, Value: v, At: at.UTC(), Mutation: mutation})
	if len(s.history) > FlagHistoryLimit {
		s.history = s.history[len(s.history)-FlagHistoryLimit:]
	}
}

// hiddenOutcome is what the gate did with a resolved hidden-session event.
type hiddenOutcome int

const (
	hiddenDelivered hiddenOutcome = iota
	hiddenSuppressed
	hiddenWouldSuppress
)

// hiddenDecision is one resolved hidden-session decision. On is the kind's
// effective flag value when it was taken.
type hiddenDecision struct {
	Kind, Class string
	Outcome     hiddenOutcome
	On, Probe   bool
}

func (s *Stats) recordHidden(d hiddenDecision) {
	kind, class, on := d.Kind, d.Class, d.On
	s.mu.Lock()
	b := s.curBucket
	b.seen++
	if on {
		b.whileOn++
	}
	switch d.Outcome {
	case hiddenSuppressed:
		b.routineWhileOn++
		b.counts[statKey{StatSuppressed, kind, class}]++
	case hiddenWouldSuppress:
		b.counts[statKey{StatWouldSuppress, kind, class}]++
	default:
		b.counts[statKey{StatHiddenDelivered, kind, class}]++
		if on && class != "routine" {
			b.counts[statKey{StatDeliveredWhileOn, kind, class}]++
			if d.Probe {
				b.counts[statKey{StatProbeDeliveredWhileOn, kind, class}]++
			}
		}
	}
	s.mu.Unlock()
}

// recordUnresolved counts a fail-open delivery for an unresolved session.
func (s *Stats) recordUnresolved(class string) {
	s.mu.Lock()
	s.curBucket.counts[statKey{StatUnresolved, string(KindUnresolved), class}]++
	s.mu.Unlock()
}
