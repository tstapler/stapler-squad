package prwatch

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRingBuffer_should_KeepLast200DispositionsWithoutActorOrText_When_ManyEventsDisposed(t *testing.T) {
	var rb RingBuffer
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	drops := AllDropReasons()
	for i := 0; i < 1000; i++ {
		d := Disposition{At: base.Add(time.Duration(i) * time.Second), Kind: KindComment, Repo: "o/r", PR: i, SessionUUID: "s"}
		switch i % 3 {
		case 0:
			d.Disposition = DispositionSteered
			d.ActorClass = ActorOther
		case 1:
			d.Disposition = DispositionHeld
			d.Reason = string(HoldWindowBatching)
		default:
			d.Disposition = DispositionDropped
			d.Reason = string(drops[i%len(drops)])
		}
		rb.Record(d)
	}
	got := rb.Entries()
	if len(got) != 200 || rb.Len() != 200 {
		t.Fatalf("len = %d/%d, want 200", len(got), rb.Len())
	}
	if got[0].PR != 800 || got[199].PR != 999 {
		t.Fatalf("window = PR %d..%d, want 800..999", got[0].PR, got[199].PR)
	}
	for i := 1; i < len(got); i++ {
		if !got[i].At.After(got[i-1].At) {
			t.Fatalf("entries not oldest-first at %d", i)
		}
	}
	raw, _ := json.Marshal(got)
	for _, banned := range []string{"hostile-actor", "SECRET SUMMARY", "login"} {
		if strings.Contains(string(raw), banned) {
			t.Fatalf("ring buffer leaked %q", banned)
		}
	}
}

func TestRingBuffer_should_ReturnOldestFirst_When_NotYetFull(t *testing.T) {
	var rb RingBuffer
	for i := 1; i <= 3; i++ {
		rb.Record(Disposition{PR: i})
	}
	got := rb.Entries()
	if len(got) != 3 || got[0].PR != 1 || got[2].PR != 3 {
		t.Fatalf("got %+v", got)
	}
}
