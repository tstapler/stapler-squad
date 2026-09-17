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

func TestAltScreenTracker_Observe_should_ReturnChangedFalse_When_NoMarkersPresentOrMarkersSplitAcrossCalls(t *testing.T) {
	t.Run("no markers present", func(t *testing.T) {
		var tracker AltScreenTracker
		active, changed := tracker.Observe("just some plain output\r\n")
		if active || changed {
			t.Fatalf("Observe(no markers) = (active=%v, changed=%v), want (false, false)", active, changed)
		}
	})

	t.Run("markers split across two Observe calls", func(t *testing.T) {
		var tracker AltScreenTracker
		// Split the enter sequence's bytes across two calls -- since each
		// Observe call only sees a complete literal marker or none at all,
		// a marker whose bytes straddle a chunk boundary is invisible to
		// either call, and the tracker's state must simply persist.
		active, changed := tracker.Observe("\x1b[?10")
		if active || changed {
			t.Fatalf("Observe(partial marker 1) = (active=%v, changed=%v), want (false, false)", active, changed)
		}
		active, changed = tracker.Observe("49h")
		if active || changed {
			t.Fatalf("Observe(partial marker 2) = (active=%v, changed=%v), want (false, false)", active, changed)
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
