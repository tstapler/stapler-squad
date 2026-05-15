package analytics

import (
	"context"
	"testing"
	"time"
)

func TestMangleCorrelator_NoMangle(t *testing.T) {
	c := NewMangleCorrelator(5*time.Second, 100)
	c.RecordStage1("sess1", 100, "abc123", 20)
	mangled, mtype := c.CheckStage2("sess1", 100, "abc123", 20)
	if mangled || mtype != "" {
		t.Errorf("expected no mangle, got mangled=%v mtype=%q", mangled, mtype)
	}
}

func TestMangleCorrelator_Truncated(t *testing.T) {
	c := NewMangleCorrelator(5*time.Second, 100)
	c.RecordStage1("sess1", 200, "abc123", 20)
	mangled, mtype := c.CheckStage2("sess1", 200, "xyz789", 10)
	if !mangled || mtype != "truncated" {
		t.Errorf("expected truncated mangle, got mangled=%v mtype=%q", mangled, mtype)
	}
}

func TestMangleCorrelator_Mutated(t *testing.T) {
	c := NewMangleCorrelator(5*time.Second, 100)
	c.RecordStage1("sess1", 300, "abc123", 20)
	mangled, mtype := c.CheckStage2("sess1", 300, "xyz789", 20)
	if !mangled || mtype != "mutated" {
		t.Errorf("expected mutated mangle, got mangled=%v mtype=%q", mangled, mtype)
	}
}

func TestMangleCorrelator_Stripped(t *testing.T) {
	spy := &spyWriter{}
	c := NewMangleCorrelator(100*time.Millisecond, 100)
	c.RecordStage1("sess1", 400, "abc123", 20)
	// Wait for TTL to expire
	time.Sleep(200 * time.Millisecond)
	c.EvictExpired(context.Background(), spy)
	if len(spy.events) != 1 || !spy.events[0].Mangled || spy.events[0].MangleType != "stripped" {
		t.Errorf("expected 1 stripped event, got %+v", spy.events)
	}
}

func TestMangleCorrelator_BoundedSize(t *testing.T) {
	c := NewMangleCorrelator(5*time.Second, 3)
	c.RecordStage1("s", 1, "h1", 10)
	c.RecordStage1("s", 2, "h2", 10)
	c.RecordStage1("s", 3, "h3", 10)
	c.RecordStage1("s", 4, "h4", 10) // should evict one
	if c.PendingCount() > 3 {
		t.Errorf("expected at most 3 pending, got %d", c.PendingCount())
	}
}

func TestMangleCorrelator_MissingStage1(t *testing.T) {
	c := NewMangleCorrelator(5*time.Second, 100)
	mangled, mtype := c.CheckStage2("sess_never_seen", 999, "abc", 10)
	if mangled || mtype != "" {
		t.Errorf("expected no mangle for unknown seq, got mangled=%v mtype=%q", mangled, mtype)
	}
}
