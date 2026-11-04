package waivers

import (
	"strings"
	"testing"
	"time"
)

var today = time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)

func w(row, reason, granted, expires string) Waiver { return Waiver{row, reason, granted, expires} }

const goodReason = "owner accepts lower hit ratio on cold CI"

func TestValidCases(t *testing.T) {
	ok, bad := Validate([]Waiver{w("P10.hit_ratio_floor", goodReason, "2026-11-01", "2027-01-15")}, today)
	if len(ok) != 1 || len(bad) != 0 {
		t.Fatalf("%v %v", ok, bad)
	}
}

func TestRejections(t *testing.T) {
	cases := map[string]Waiver{
		"empty reason":         w("P10.hit_ratio_floor", "", "2026-11-01", "2026-12-01"),
		"short reason":         w("P10.hit_ratio_floor", "too short", "2026-11-01", "2026-12-01"),
		"120 day expiry":       w("P10.disk_ceiling", goodReason, "2026-11-01", "2027-03-01"),
		"expired":              w("P10.disk_ceiling", goodReason, "2026-06-01", "2026-09-01"),
		"waiver on P5":         w("P5", goodReason, "2026-11-01", "2026-12-01"),
		"cold row":             w("P10.cold", goodReason, "2026-11-01", "2026-12-01"),
		"bad date":             w("P10.disk_ceiling", goodReason, "yesterday", "2026-12-01"),
		"expires before grant": w("P10.disk_ceiling", goodReason, "2026-11-01", "2026-10-01"),
	}
	for name, c := range cases {
		ok, bad := Validate([]Waiver{c}, today)
		if len(ok) != 0 || len(bad) != 1 {
			t.Errorf("%s: ok=%v bad=%v", name, ok, bad)
		}
	}
}

func TestExactly90DaysAllowed(t *testing.T) {
	if ok, _ := Validate([]Waiver{w("P10.disk_ceiling", goodReason, "2026-11-01", "2027-01-30")}, today); len(ok) != 1 {
		t.Fatal("90 days is the limit and must pass")
	}
}

func TestParseIgnoresOtherKeysAndRendersBlock(t *testing.T) {
	doc := "interview_date: x\neconomic_gate:\n  decision: continue\nwaivers:\n  - row: P10.hit_ratio_floor\n    reason: " + goodReason + "\n    granted: 2026-11-01\n    expires: 2026-12-01\n"
	ws, err := Parse(strings.NewReader(doc))
	if err != nil || len(ws) != 1 {
		t.Fatalf("%v %v", ws, err)
	}
	ok, _ := Validate(ws, today)
	if !strings.HasPrefix(Block(ok), "WAIVER P10.hit_ratio_floor: ") || !strings.Contains(Inline(ok), "until 2026-12-01") {
		t.Fatalf("%q %q", Block(ok), Inline(ok))
	}
	if _, found := Active(ok, "P10.hit_ratio_floor"); !found {
		t.Fatal("active")
	}
}

func TestParseNoWaiversKey(t *testing.T) {
	ws, err := Parse(strings.NewReader("a: 1\n"))
	if err != nil || len(ws) != 0 {
		t.Fatalf("%v %v", ws, err)
	}
	if ws, err := Parse(strings.NewReader("")); err != nil || len(ws) != 0 {
		t.Fatalf("empty doc: %v %v", ws, err)
	}
}
