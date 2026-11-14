package instance

import (
	"strings"
	"sync"
	"testing"

	"github.com/tstapler/stapler-squad/session/claudectl"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ptyio"
)

// TestInstance_Preview_DoesNotReturnStaleUnboundedHistory is the regression test for the
// bug where Preview() called ctrl.GetRecentOutput(0), which PTYAccess/CircularBuffer treat
// as "return the entire buffer" rather than "return nothing"/"return a bounded recent
// window." Because the buffer retains up to 10MB of raw PTY output for the session's whole
// lifetime, a dialog answered (and visibly gone) long ago would still appear in Preview()'s
// output until 10MB of subsequent output evicted it — causing detection logic like
// isStartupDialog/shouldApprovePrompt to keep firing on a prompt the user had already
// dismissed. Preview() must only see a bounded recent tail, matching its own doc comment
// ("current visible terminal content").
func TestInstance_Preview_DoesNotReturnStaleUnboundedHistory(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "stale-preview-session", Status: domain.Active}
	inst.started.Store(true)

	buf := ptyio.NewCircularBuffer(1024 * 1024)
	dialogText := "Do you trust the files in this folder?\n1. Yes, proceed\n2. No, exit"
	if _, err := buf.Write([]byte(dialogText)); err != nil {
		t.Fatalf("write dialog text: %v", err)
	}

	// Simulate enough subsequent terminal output for the dialog to have scrolled well
	// past the bounded tail window that Preview() should now use, even though it is
	// still well short of the 10MB buffer capacity that the old unbounded read relied on
	// eventually evicting.
	filler := strings.Repeat("x", detection.StatusDetectionTailBytes*2)
	if _, err := buf.Write([]byte(filler)); err != nil {
		t.Fatalf("write filler: %v", err)
	}

	pa := ptyio.NewPTYAccess(inst.Title, nil, buf)
	ctrl := claudectl.NewBareForTest(inst.Title, inst)
	ctrl.SetPTYAccessForTest(pa)
	inst.controllerManager.SetController(ctrl)

	content, err := inst.Preview()
	if err != nil {
		t.Fatalf("Preview() error: %v", err)
	}

	if strings.Contains(content, "Do you trust the files in this folder?") {
		t.Fatalf("Preview() returned stale dialog text long after it scrolled out of view: %q", content)
	}
	if !strings.HasSuffix(content, filler[len(filler)-10:]) {
		t.Fatalf("Preview() should return the recent tail of filler output, got: %q", content)
	}
}

// TestInstance_GetAltScreenActive_should_ReflectTrackerObservation_When_ReadConcurrentlyWithPTYWrite
// is the regression guard named by .claude/rules/instance-lock-free-reads.md:
// GetAltScreenActive's lock-free Snapshot() read must not race a concurrent
// setAltScreenActiveLocked write reached via ObserveAltScreenTransition. Run
// with -race; the value observed after the writer goroutine finishes must
// reflect the last transition it applied ("\x1b[?1049l" -> false).
func TestInstance_GetAltScreenActive_should_ReflectTrackerObservation_When_ReadConcurrentlyWithPTYWrite(t *testing.T) {
	inst := &Instance{Title: "altscreen-race-session"}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		inst.ObserveAltScreenTransition([]byte("\x1b[?1049h"))
		inst.ObserveAltScreenTransition([]byte("\x1b[?1049l"))
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = inst.GetAltScreenActive()
		}
	}()
	wg.Wait()

	if inst.GetAltScreenActive() {
		t.Fatalf("GetAltScreenActive() = true after exit sequence, want false")
	}
}

// TestInstance_GetAltScreenBootstrapped distinguishes "confirmed not in alt
// screen" from "never checked" -- without this flag,
// altScreenActiveForSnapshot (server/services/connectrpc_websocket.go) can't
// tell the two apart and re-runs a real tmux subprocess query
// (IsAlternateScreenActiveBootstrap) on every connect/resize for any session
// that's genuinely not in alt screen, instead of caching the confirmed answer
// after the first check.
func TestInstance_GetAltScreenBootstrapped_should_ReturnFalse_When_NeverObservedOrBootstrapped(t *testing.T) {
	inst := &Instance{Title: "altscreen-bootstrap-session"}

	if inst.GetAltScreenBootstrapped() {
		t.Fatalf("GetAltScreenBootstrapped() = true before any observation or bootstrap, want false")
	}
}

func TestInstance_GetAltScreenBootstrapped_should_ReturnTrue_When_SetAltScreenActiveBootstrapCalled(t *testing.T) {
	inst := &Instance{Title: "altscreen-bootstrap-session"}

	inst.SetAltScreenActiveBootstrap(false)

	if !inst.GetAltScreenBootstrapped() {
		t.Fatalf("GetAltScreenBootstrapped() = false after SetAltScreenActiveBootstrap, want true")
	}
	if inst.GetAltScreenActive() {
		t.Fatalf("GetAltScreenActive() = true after SetAltScreenActiveBootstrap(false), want false")
	}
}

func TestInstance_GetAltScreenBootstrapped_should_ReturnTrue_When_ObserveAltScreenTransitionChangesState(t *testing.T) {
	inst := &Instance{Title: "altscreen-bootstrap-session"}

	inst.ObserveAltScreenTransition([]byte("\x1b[?1049h"))

	if !inst.GetAltScreenBootstrapped() {
		t.Fatalf("GetAltScreenBootstrapped() = false after a live-observed transition, want true")
	}
}
