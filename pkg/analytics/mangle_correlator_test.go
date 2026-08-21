package analytics

import (
	"context"
	"testing"
	"time"
)

func TestMangleCorrelator_NoMangle(t *testing.T) {
	c := NewMangleCorrelator(5*time.Second, 100)
	c.RecordStage1("sess1", "SGR", "abc123", 20)
	mangled, mtype := c.CheckStage2("sess1", "SGR", "abc123", 20)
	if mangled || mtype != "" {
		t.Errorf("expected no mangle, got mangled=%v mtype=%q", mangled, mtype)
	}
}

func TestMangleCorrelator_Truncated(t *testing.T) {
	c := NewMangleCorrelator(5*time.Second, 100)
	c.RecordStage1("sess1", "OSC", "abc123", 20)
	mangled, mtype := c.CheckStage2("sess1", "OSC", "xyz789", 10)
	if !mangled || mtype != "truncated" {
		t.Errorf("expected truncated mangle, got mangled=%v mtype=%q", mangled, mtype)
	}
}

func TestMangleCorrelator_Mutated(t *testing.T) {
	c := NewMangleCorrelator(5*time.Second, 100)
	c.RecordStage1("sess1", "SGR", "abc123", 20)
	mangled, mtype := c.CheckStage2("sess1", "SGR", "xyz789", 20)
	if !mangled || mtype != "mutated" {
		t.Errorf("expected mutated mangle, got mangled=%v mtype=%q", mangled, mtype)
	}
}

func TestMangleCorrelator_Stripped(t *testing.T) {
	spy := &spyWriter{}
	c := NewMangleCorrelator(100*time.Millisecond, 100)
	c.RecordStage1("sess1", "SGR", "abc123", 20)
	// Wait for TTL to expire
	time.Sleep(200 * time.Millisecond)
	c.EvictExpired(context.Background(), spy)
	if len(spy.events) != 1 || !spy.events[0].Mangled || spy.events[0].MangleType != "stripped" {
		t.Errorf("expected 1 stripped event, got %+v", spy.events)
	}
	if spy.events[0].SequenceType != "SGR" {
		t.Errorf("expected stripped event to carry the original sequence type, got %q", spy.events[0].SequenceType)
	}
}

func TestMangleCorrelator_BoundedSize(t *testing.T) {
	c := NewMangleCorrelator(5*time.Second, 3)
	c.RecordStage1("s", "SGR", "h1", 10)
	c.RecordStage1("s", "SGR", "h2", 10)
	c.RecordStage1("s", "SGR", "h3", 10)
	c.RecordStage1("s", "SGR", "h4", 10) // should evict one
	if c.PendingCount() > 3 {
		t.Errorf("expected at most 3 pending, got %d", c.PendingCount())
	}
}

func TestMangleCorrelator_MissingStage1(t *testing.T) {
	c := NewMangleCorrelator(5*time.Second, 100)
	mangled, mtype := c.CheckStage2("sess_never_seen", "SGR", "abc", 10)
	if mangled || mtype != "" {
		t.Errorf("expected no mangle for unknown seq, got mangled=%v mtype=%q", mangled, mtype)
	}
}

// TestMangleCorrelator_OrdinalPerType verifies that ordinals are tracked independently per
// (session, sequenceType) pair — a Stage 1 Cursor sequence interleaved between two SGR
// sequences doesn't shift the SGR ordinal numbering, so SGR#1 still matches SGR#1 and
// SGR#2 still matches SGR#2 even though the two sides observed a different relative
// ordering of *different* types. This is the core robustness property of the ordinal
// redesign: correlation only requires order-preservation *within* a sequence type, not
// across all types combined — which tolerates exactly the kind of interleaving difference
// that two independently-coalescing transports (raw PTY reads vs. control-mode frames)
// can legitimately introduce.
func TestMangleCorrelator_OrdinalPerType(t *testing.T) {
	c := NewMangleCorrelator(5*time.Second, 100)

	// Stage 1 order: SGR(red), Cursor(up), SGR(green)
	c.RecordStage1("sess1", "SGR", "hash-red", 5)
	c.RecordStage1("sess1", "Cursor", "hash-up", 3)
	c.RecordStage1("sess1", "SGR", "hash-green", 5)

	// Stage 2 order differs across types (Cursor arrives first), but SGR-to-SGR order
	// is preserved.
	if mangled, _ := c.CheckStage2("sess1", "Cursor", "hash-up", 3); mangled {
		t.Errorf("Cursor#1 should match despite interleaving, got mangled=true")
	}
	if mangled, _ := c.CheckStage2("sess1", "SGR", "hash-red", 5); mangled {
		t.Errorf("SGR#1 should match hash-red, got mangled=true")
	}
	if mangled, _ := c.CheckStage2("sess1", "SGR", "hash-green", 5); mangled {
		t.Errorf("SGR#2 should match hash-green, got mangled=true")
	}
}

// TestMangleCorrelator_EvictExpired_PrunesStaleOrdinalCounters is the PerfFix-2 regression
// test: stage1Ordinals/stage2Ordinals previously had no TTL of their own, so a session's
// ordinal counters for a (session, type) pair survived forever once EvictExpired had nothing
// left in pending to reap for that pair — confirmed as the #1 live-heap consumer
// (25.18% inuse_space) before ordinalLastSeen-driven pruning was added.
func TestMangleCorrelator_EvictExpired_PrunesStaleOrdinalCounters(t *testing.T) {
	spy := &spyWriter{}
	c := NewMangleCorrelator(100*time.Millisecond, 100)

	c.RecordStage1("sess1", "SGR", "hash-1", 5)
	c.CheckStage2("sess1", "SGR", "hash-1", 5)

	ok := ordinalKey{"sess1", "SGR"}
	c.mu.Lock()
	_, s1ok := c.stage1Ordinals[ok]
	_, s2ok := c.stage2Ordinals[ok]
	c.mu.Unlock()
	if !s1ok || !s2ok {
		t.Fatalf("expected ordinal counters to be present immediately after use, s1=%v s2=%v", s1ok, s2ok)
	}

	time.Sleep(200 * time.Millisecond)
	c.EvictExpired(context.Background(), spy)

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.stage1Ordinals[ordinalKey{"sess1", "SGR"}]; ok {
		t.Error("expected stage1Ordinals entry to be pruned once its (session, type) pair went quiet")
	}
	if _, ok := c.stage2Ordinals[ordinalKey{"sess1", "SGR"}]; ok {
		t.Error("expected stage2Ordinals entry to be pruned once its (session, type) pair went quiet")
	}
	if _, ok := c.ordinalLastSeen[ordinalKey{"sess1", "SGR"}]; ok {
		t.Error("expected ordinalLastSeen entry to be pruned once its (session, type) pair went quiet")
	}
}

// TestMangleCorrelator_DroppedSequenceDesyncsSubsequentOrdinals documents the known
// limitation of ordinal correlation: an actual dropped sequence of a given type shifts
// every subsequent ordinal for that (session, type) out of alignment. This is not a
// "passing" test in the sense of proving correctness — it demonstrates the trade-off
// accepted by this design (robust to timing drift, not to an actual drop) so a future
// reader can see the concrete failure mode rather than rediscovering it.
func TestMangleCorrelator_DroppedSequenceDesyncsSubsequentOrdinals(t *testing.T) {
	c := NewMangleCorrelator(5*time.Second, 100)

	c.RecordStage1("sess1", "SGR", "hash-1", 5)
	c.RecordStage1("sess1", "SGR", "hash-2", 5) // this one is "dropped" — Stage 2 never sees it
	c.RecordStage1("sess1", "SGR", "hash-3", 5)

	// Stage 2 only observes hash-1 and hash-3 (hash-2 was genuinely stripped in transit).
	if mangled, _ := c.CheckStage2("sess1", "SGR", "hash-1", 5); mangled {
		t.Errorf("first observed sequence should still match its ordinal")
	}
	// The second Stage 2 call consumes SGR ordinal #2, which was hash-2 — not hash-3.
	mangled, mtype := c.CheckStage2("sess1", "SGR", "hash-3", 5)
	if !mangled || mtype != "mutated" {
		t.Errorf("expected the desync to surface as a mismatch (mutated) against the dropped "+
			"ordinal's hash, got mangled=%v mtype=%q — this documents the accepted trade-off, "+
			"not a bug in this test", mangled, mtype)
	}
}
