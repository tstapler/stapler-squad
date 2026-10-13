package session

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/domain"
)

const (
	// stopTailMaxLines and stopTailMaxBytes bound the pane tail kept in a StopSnapshot.
	stopTailMaxLines = 40
	stopTailMaxBytes = 4096
)

// BoundedStopTail returns the last stopTailMaxLines lines of raw, ANSI-stripped,
// with trailing blank lines trimmed and at most stopTailMaxBytes bytes. The
// result is always valid UTF-8: invalid bytes are dropped and the byte cut
// advances to a rune boundary. It does not redact; see the service-side
// sanitizer before sending the result to any durable sink.
func BoundedStopTail(raw string) string {
	s := stripANSI([]byte(raw))
	s = strings.ToValidUTF8(s, "")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.TrimRight(s, " \t\n")
	if s == "" {
		return ""
	}

	if lines := strings.Split(s, "\n"); len(lines) > stopTailMaxLines {
		s = strings.Join(lines[len(lines)-stopTailMaxLines:], "\n")
	}

	return TailBytes(s, stopTailMaxBytes)
}

// TailBytes keeps the last n bytes of s, advancing the cut forward to a rune
// boundary so valid UTF-8 input stays valid.
func TailBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := len(s) - n
	for cut < len(s) && !utf8.RuneStart(s[cut]) {
		cut++
	}
	return s[cut:]
}

// stopCaptureTimeout bounds the whole pre-teardown capture so a hung tmux
// call can never delay the completion callback.
const stopCaptureTimeout = 2 * time.Second

// stopSource is what a driver reads to build a StopSnapshot. The Instance
// implementation is instanceStopSource; tests substitute a fake.
type stopSource interface {
	Status() (detection.DetectedStatus, string)
	LastActivity() time.Time
	TmuxAlive() bool
	ControllerActive() bool
	PaneExit() (dead bool, code int)
	Tail(ctx context.Context) (string, error)
}

type instanceStopSource struct {
	inst       *Instance
	controller *ClaudeController
}

func (s instanceStopSource) Status() (detection.DetectedStatus, string) {
	if s.controller == nil {
		return detection.StatusUnknown, ""
	}
	return s.controller.GetCurrentStatus()
}
func (s instanceStopSource) LastActivity() time.Time { return s.inst.LastMeaningfulOutputTime() }
func (s instanceStopSource) TmuxAlive() bool         { return s.inst.TmuxAlive() }
func (s instanceStopSource) ControllerActive() bool {
	return s.controller != nil && s.controller.IsStarted()
}
func (s instanceStopSource) PaneExit() (bool, int) {
	dead, code, _ := s.inst.PaneExitInfo()
	return dead, code
}
func (s instanceStopSource) Tail(ctx context.Context) (string, error) {
	return s.inst.PreviewContext(ctx)
}

// bucketFor folds a detector result into the few classes the classifier needs.
// StatusReady is dead code in the detector (never produced), so it is treated
// like any non-safe idle rather than as a prompt that is safe to type into.
func bucketFor(status detection.DetectedStatus, statusContext string) domain.StatusBucket {
	switch {
	case IsSafeSteerStatus(status, statusContext):
		return domain.BucketSafeIdle
	}
	switch status {
	case detection.StatusIdle, detection.StatusReady, detection.StatusSuccess:
		return domain.BucketIdle
	case detection.StatusNeedsApproval, detection.StatusInputRequired:
		return domain.BucketNeedsInput
	case detection.StatusError, detection.StatusTestsFailing:
		return domain.BucketError
	case detection.StatusExecuting, detection.StatusProcessing,
		detection.StatusWaitingForAgent, detection.StatusCompacting:
		return domain.BucketActive
	default:
		return domain.BucketUnknown
	}
}

// captureStopSnapshot reads src once, before the driver tears anything down.
// It never panics and never blocks past timeout; any failure is recorded in
// CaptureErr and the partial snapshot is still returned.
func captureStopSnapshot(src stopSource, timeout time.Duration) domain.StopSnapshot {
	if timeout <= 0 {
		timeout = stopCaptureTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resCh := make(chan domain.StopSnapshot, 1)
	go func() {
		var snap domain.StopSnapshot
		defer func() {
			if r := recover(); r != nil {
				snap.CaptureErr = fmt.Sprintf("capture panicked: %v", r)
			}
			resCh <- snap
		}()
		snap = readStopSnapshot(ctx, src)
	}()

	select {
	case snap := <-resCh:
		return snap
	case <-ctx.Done():
		return domain.StopSnapshot{CapturedAt: time.Now(), CaptureErr: "capture timed out"}
	}
}

func readStopSnapshot(ctx context.Context, src stopSource) domain.StopSnapshot {
	snap := domain.StopSnapshot{CapturedAt: time.Now()}
	status, statusCtx := src.Status()
	snap.Bucket = bucketFor(status, statusCtx)
	snap.DetectedStatus = status.String()
	snap.DetectedContext = statusCtx
	snap.LastActivityAt = src.LastActivity()
	snap.TmuxAlive = src.TmuxAlive()
	snap.ControllerActive = src.ControllerActive()
	snap.PaneDead, snap.ExitCode = src.PaneExit()
	tail, err := src.Tail(ctx)
	if err != nil {
		snap.CaptureErr = "tail: " + err.Error()
	}
	snap.PaneTail = BoundedStopTail(tail)
	return snap
}
