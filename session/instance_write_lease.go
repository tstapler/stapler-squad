package session

// instance_write_lease.go is the per-instance TerminalWriteLease (plan Story
// 5.0): one exclusive, non-reentrant lease per instance that every automated
// or unary pane writer holds for the duration of its write, so a driver's
// answer key, a nudge, a steer and (later) a Reply never interleave bytes.
//
// The lease is a passed capability. Exactly one function per call chain
// acquires it (TryTerminalWriteLease / AcquireTerminalWriteLease); the write
// primitives (SubmitDriverContent, SubmitContentWithEnter, SendKeysWithTimeout)
// only receive it, and the innermost function that issues the write releases
// it exactly once on every return path, in the goroutine that issues the write.
// A forced release is deliberately not offered: it would allow exactly the
// interleave the lease exists to stop (a wedged write is surfaced instead,
// see ScanWedgedLeases).

import (
	"context"
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
)

// Writer names: the closed label set of the lease metrics.
const (
	LeaseWriterReply      = "reply"
	LeaseWriterDriver     = "driver"
	LeaseWriterAutonomous = "autonomous"
	LeaseWriterNudge      = "nudge"
	LeaseWriterSteer      = "steer"
	LeaseWriterMCP        = "mcp"
	LeaseWriterRateLimit  = "ratelimit"
	LeaseWriterOther      = "other"
)

const (
	// LeaseWedgeWarnAfter is how long a lease may be held before it is treated
	// as wedged (INFERRED: a healthy write, settle wait and retry takes under 10s).
	LeaseWedgeWarnAfter = 30 * time.Second
	// LeaseWedgeWarnEvery rate-limits the WARN for one wedged acquisition.
	LeaseWedgeWarnEvery = time.Minute
	// AutonomousTurnLeaseWait bounds the autonomous turn's blocking acquire
	// (INFERRED: a Reply holds the lease for at most its send timeout).
	AutonomousTurnLeaseWait = 15 * time.Second
	// RateLimitRecoveryLeaseWait bounds the rate-limit recovery input's acquire;
	// the recovery loop itself does not retry a send error.
	RateLimitRecoveryLeaseWait = 10 * time.Second
)

var (
	// ErrLeaseBusy is the retryable result while another writer holds the lease.
	ErrLeaseBusy = errors.New("a write to this session is in progress")
	// ErrNoLease is returned by a write primitive given a nil or zero-value lease.
	ErrNoLease = errors.New("terminal write lease is required")
	// ErrLeaseMismatch is returned by a write primitive given a lease for another instance.
	ErrLeaseMismatch = errors.New("terminal write lease belongs to another session")
)

// LeaseFlag is the injected atomic behind the terminal_write_lease flag. The
// zero value (and a nil pointer) is "on": an unreadable config keeps the lease
// on, so only an explicit persisted false ever turns serialization off.
type LeaseFlag struct{ off atomic.Bool }

// Enabled reports whether leases are exclusive.
func (f *LeaseFlag) Enabled() bool { return f == nil || !f.off.Load() }

// SetEnabled flips the flag; false makes every lease non-exclusive.
func (f *LeaseFlag) SetEnabled(on bool) { f.off.Store(!on) }

// DefaultLeaseFlag is the process-wide flag instances read unless one was
// injected with SetLeaseFlag.
var DefaultLeaseFlag = &LeaseFlag{}

// leaseOwner is satisfied by *Instance and by the pane fakes in tests, so a
// primitive can compare a lease's instance with the pane it is about to write.
type leaseOwner interface {
	LeaseOwnerUUID() string
}

// HeldLease is a held write lease. Only the constructors on *Instance create
// one; a nil pointer or a zero value is refused by every write primitive.
type HeldLease struct {
	owner      string
	writer     string
	acquiredAt time.Time
	id         uint64
	state      *writeLeaseState // nil for a non-exclusive lease (flag off)
	inst       *Instance
	once       sync.Once

	// wedge bookkeeping, guarded by leaseRegistry.mu
	wedgeSeen bool
	lastWarn  time.Time
}

// Writer names who holds the lease (one of the LeaseWriter* constants).
func (l *HeldLease) Writer() string {
	if l == nil {
		return ""
	}
	return l.writer
}

// Release frees the lease. Idempotent and nil-safe: the one-release rule is the
// design, the sync.Once is the backstop.
func (l *HeldLease) Release() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		if l.state == nil {
			return
		}
		leaseRegistry.remove(l)
		<-l.state.slot
	})
}

// check refuses a nil, zero-value or foreign lease. It has no side effects, so
// a refused primitive performs 0 writes.
func (l *HeldLease) check(target leaseOwner) error {
	if l == nil || l.owner == "" {
		return ErrNoLease
	}
	if target.LeaseOwnerUUID() != l.owner {
		return ErrLeaseMismatch
	}
	return nil
}

// writeLeaseState is the per-instance semaphore; the zero value is usable.
type writeLeaseState struct {
	once  sync.Once
	slot  chan struct{} // cap 1: a token in the channel means the lease is held
	owner string
	flag  atomic.Pointer[LeaseFlag]
}

func (i *Instance) leaseState() *writeLeaseState {
	st := &i.writeLease
	st.once.Do(func() {
		st.slot = make(chan struct{}, 1)
		st.owner = i.UUID
		if st.owner == "" {
			st.owner = uuid.NewString()
		}
	})
	return st
}

// LeaseOwnerUUID is the immutable identity a lease is bound to (the instance
// UUID, or a generated one for an instance that has none).
func (i *Instance) LeaseOwnerUUID() string { return i.leaseState().owner }

// SetLeaseFlag injects the flag this instance consults (nil restores the default).
func (i *Instance) SetLeaseFlag(f *LeaseFlag) { i.leaseState().flag.Store(f) }

func (st *writeLeaseState) exclusive() bool {
	f := st.flag.Load()
	if f == nil {
		f = DefaultLeaseFlag
	}
	return f.Enabled()
}

// nonExclusiveLease is what the flag-off path hands out: a valid capability
// that serializes nothing.
func (i *Instance) nonExclusiveLease(st *writeLeaseState, writer string) *HeldLease {
	return &HeldLease{owner: st.owner, writer: normalizeLeaseWriter(writer), inst: i}
}

func (i *Instance) exclusiveLease(st *writeLeaseState, writer string) *HeldLease {
	l := i.nonExclusiveLease(st, writer)
	l.state = st
	l.acquiredAt = time.Now()
	leaseRegistry.add(l)
	return l
}

// TryTerminalWriteLease acquires the lease without waiting. With the
// terminal_write_lease flag off it always succeeds with a non-exclusive lease
// (today's behavior: the capability types still work, nothing is serialized).
func (i *Instance) TryTerminalWriteLease(writer string) (*HeldLease, bool) {
	st := i.leaseState()
	if !st.exclusive() {
		return i.nonExclusiveLease(st, writer), true
	}
	select {
	case st.slot <- struct{}{}:
		return i.exclusiveLease(st, writer), true
	default:
		recordLeaseBusy(writer)
		return nil, false
	}
}

// AcquireTerminalWriteLease waits up to maxWait for the lease. It returns
// ErrLeaseBusy on timeout and ctx.Err() on cancellation.
func (i *Instance) AcquireTerminalWriteLease(ctx context.Context, writer string, maxWait time.Duration) (*HeldLease, error) {
	st := i.leaseState()
	if !st.exclusive() {
		return i.nonExclusiveLease(st, writer), nil
	}
	select {
	case st.slot <- struct{}{}:
		return i.exclusiveLease(st, writer), nil
	default:
	}
	if maxWait <= 0 {
		recordLeaseBusy(writer)
		return nil, ErrLeaseBusy
	}
	timer := time.NewTimer(maxWait)
	defer timer.Stop()
	select {
	case st.slot <- struct{}{}:
		return i.exclusiveLease(st, writer), nil
	case <-timer.C:
		recordLeaseBusy(writer)
		return nil, ErrLeaseBusy
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type leaseTB interface {
	Helper()
	Fatalf(format string, args ...any)
}

// AssertLeaseFree ends the test of every acquirer: a leaked lease is a
// permanent busy, so the lease must be takeable at once on every exit path.
func AssertLeaseFree(t leaseTB, inst *Instance) {
	t.Helper()
	l, ok := inst.TryTerminalWriteLease(LeaseWriterOther)
	if !ok {
		t.Fatalf("terminal write lease is still held: an acquirer returned without releasing it")
		return
	}
	l.Release()
}

// leaseRegistry tracks every exclusive lease currently held, for the held-seconds
// gauge and the wedge scan. It is never held across a call out.
var leaseRegistry = &leaseRegistryT{held: map[uint64]*HeldLease{}}

type leaseRegistryT struct {
	mu     sync.Mutex
	held   map[uint64]*HeldLease
	nextID uint64
}

func (r *leaseRegistryT) add(l *HeldLease) {
	r.mu.Lock()
	r.nextID++
	l.id = r.nextID
	r.held[l.id] = l
	r.mu.Unlock()
}

func (r *leaseRegistryT) remove(l *HeldLease) {
	r.mu.Lock()
	delete(r.held, l.id)
	r.mu.Unlock()
}

// WedgedLease is one wedge episode: a single acquisition that outlived
// LeaseWedgeWarnAfter.
type WedgedLease struct {
	Instance      *Instance
	Writer        string
	Held          time.Duration
	AcquisitionID uint64
}

// ScanWedgedLeases logs the terminal_write_lease_wedged WARN for every
// exclusive lease held longer than warnAfter (at most once per
// LeaseWedgeWarnEvery per acquisition) and returns the acquisitions seen
// wedged for the first time: the once-per-episode set the server turns into a
// tray warning. It never releases anything. include restricts the scan to the
// instances it accepts (nil scans every held lease; a test passes its own
// instance so a parallel test's lease is never swept up).
func ScanWedgedLeases(now time.Time, warnAfter time.Duration, include func(*Instance) bool) []WedgedLease {
	var warns, firsts []WedgedLease
	leaseRegistry.mu.Lock()
	for _, l := range leaseRegistry.held {
		held := now.Sub(l.acquiredAt)
		if held < warnAfter || (include != nil && !include(l.inst)) {
			continue
		}
		w := WedgedLease{Instance: l.inst, Writer: l.writer, Held: held, AcquisitionID: l.id}
		if !l.wedgeSeen {
			l.wedgeSeen = true
			firsts = append(firsts, w)
		}
		if l.lastWarn.IsZero() || now.Sub(l.lastWarn) >= LeaseWedgeWarnEvery {
			l.lastWarn = now
			warns = append(warns, w)
		}
	}
	leaseRegistry.mu.Unlock()

	sort.Slice(warns, func(a, b int) bool { return warns[a].AcquisitionID < warns[b].AcquisitionID })
	for _, w := range warns {
		log.Warn("terminal_write_lease_wedged",
			"writer", w.Writer, "session", w.Instance.Snapshot().Title, "held", w.Held.Round(time.Second))
	}
	sort.Slice(firsts, func(a, b int) bool { return firsts[a].AcquisitionID < firsts[b].AcquisitionID })
	return firsts
}
