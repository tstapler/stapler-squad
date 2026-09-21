package ansi

import "testing"

func TestAltScreenTracker_Observe_should_ReturnActiveTrueChangedTrue_When_DECSET1049EnterSequenceSeen(t *testing.T) {
	var tracker AltScreenTracker

	active, changed := tracker.Observe("\x1b[?1049h")
	if !active || !changed {
		t.Fatalf("Observe(enter) = (active=%v, changed=%v), want (true, true)", active, changed)
	}

	active, changed = tracker.Observe("hello\n")
	if !active || changed {
		t.Fatalf("Observe(plain text) = (active=%v, changed=%v), want (true, false)", active, changed)
	}
}

func TestAltScreenTracker_Observe_should_ReturnChangedFalse_When_NoMarkersPresent(t *testing.T) {
	var tracker AltScreenTracker
	active, changed := tracker.Observe("just some plain output\r\n")
	if active || changed {
		t.Fatalf("Observe(no markers) = (active=%v, changed=%v), want (false, false)", active, changed)
	}
}

func TestAltScreenTracker_Observe_should_DetectMarker_When_SplitAcrossTwoCalls(t *testing.T) {
	// A real PTY read can split an 8-byte marker across two chunks. The
	// carry buffer exists so this is still detected on the call that
	// completes it, rather than silently going missing (see pkg/ansi's
	// AltScreenTracker.carry doc comment).
	t.Run("enter split", func(t *testing.T) {
		var tracker AltScreenTracker
		active, changed := tracker.Observe("\x1b[?10")
		if active || changed {
			t.Fatalf("Observe(partial marker 1) = (active=%v, changed=%v), want (false, false)", active, changed)
		}
		active, changed = tracker.Observe("49h")
		if !active || !changed {
			t.Fatalf("Observe(partial marker 2) = (active=%v, changed=%v), want (true, true)", active, changed)
		}
	})

	t.Run("exit split, one byte at a time", func(t *testing.T) {
		var tracker AltScreenTracker
		tracker.active = true

		var active, changed bool
		for _, b := range []byte(decset1049Exit) {
			active, changed = tracker.Observe(string(b))
		}
		if active || !changed {
			t.Fatalf("Observe(exit, byte-by-byte) final = (active=%v, changed=%v), want (false, true)", active, changed)
		}
	})
}

func TestAltScreenTracker_Observe_should_TrackExitAndEnterExitInOneCall(t *testing.T) {
	t.Run("exit only", func(t *testing.T) {
		var tracker AltScreenTracker
		tracker.active = true // start active, mirroring a session already in alt-screen

		active, changed := tracker.Observe("\x1b[?1049l")
		if active || !changed {
			t.Fatalf("Observe(exit) = (active=%v, changed=%v), want (false, true)", active, changed)
		}
	})

	t.Run("enter then exit in one call ends inactive with no net change", func(t *testing.T) {
		var tracker AltScreenTracker

		active, changed := tracker.Observe("\x1b[?1049h" + "some redraw" + "\x1b[?1049l")
		if active || changed {
			t.Fatalf("Observe(enter+exit) = (active=%v, changed=%v), want (false, false)", active, changed)
		}
	})

	t.Run("exit then enter in one call ends active", func(t *testing.T) {
		var tracker AltScreenTracker
		tracker.active = true

		active, changed := tracker.Observe("\x1b[?1049l" + "some redraw" + "\x1b[?1049h")
		if !active || changed {
			t.Fatalf("Observe(exit+enter) = (active=%v, changed=%v), want (true, false)", active, changed)
		}
	})
}
