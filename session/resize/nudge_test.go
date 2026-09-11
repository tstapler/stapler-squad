package resize

import (
	"errors"
	"testing"
)

// TestWithForcedRedraw_AlwaysCallsTwice is the regression test for the
// blank-pane-after-redeploy bug: a caller that only calls setSize(cols, rows)
// once, with no size change, gets a no-op from tmux and no redraw. This test
// fails against that pre-fix shape (a hypothetical single-call
// implementation) because it asserts a nudge call at (cols-1, rows) happens
// unconditionally, even when nothing else about the caller's state changed.
func TestWithForcedRedraw_AlwaysCallsTwice(t *testing.T) {
	var calls [][2]int
	setSize := func(cols, rows int) error {
		calls = append(calls, [2]int{cols, rows})
		return nil
	}

	if err := WithForcedRedraw(setSize, 80, 24); err != nil {
		t.Fatalf("WithForcedRedraw returned unexpected error: %v", err)
	}

	want := [][2]int{{79, 24}, {80, 24}}
	if len(calls) != len(want) {
		t.Fatalf("expected %d calls to setSize, got %d: %v", len(want), len(calls), calls)
	}
	for i, c := range want {
		if calls[i] != c {
			t.Errorf("call %d = %v, want %v", i, calls[i], c)
		}
	}
}

func TestWithForcedRedraw_NudgeErrorIgnored(t *testing.T) {
	var realCalled bool
	setSize := func(cols, rows int) error {
		if cols == 79 {
			return errors.New("nudge boom")
		}
		realCalled = true
		return nil
	}

	if err := WithForcedRedraw(setSize, 80, 24); err != nil {
		t.Fatalf("expected nudge error to be swallowed, got: %v", err)
	}
	if !realCalled {
		t.Fatal("expected the real (post-nudge) call to still run after the nudge call errored")
	}
}

func TestWithForcedRedraw_RealErrorPropagates(t *testing.T) {
	wantErr := errors.New("real boom")
	setSize := func(cols, rows int) error {
		if cols == 80 {
			return wantErr
		}
		return nil
	}

	if err := WithForcedRedraw(setSize, 80, 24); !errors.Is(err, wantErr) {
		t.Fatalf("got error %v, want %v", err, wantErr)
	}
}

func TestWithForcedRedraw_SkipsNudgeAtColsOne(t *testing.T) {
	var calls [][2]int
	setSize := func(cols, rows int) error {
		calls = append(calls, [2]int{cols, rows})
		return nil
	}

	if err := WithForcedRedraw(setSize, 1, 24); err != nil {
		t.Fatalf("WithForcedRedraw returned unexpected error: %v", err)
	}

	if want := [][2]int{{1, 24}}; len(calls) != len(want) || calls[0] != want[0] {
		t.Fatalf("expected a single call %v (no nudge below cols=1), got %v", want, calls)
	}
}
