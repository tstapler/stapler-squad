package tymux

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// capGracefulHandoff is the GetServerInfo capability of a daemon that can replace its own image
// in place with every pane kept. Gate on it, never on a version compare (ADR 0003 upstream).
const capGracefulHandoff = "graceful_handoff"

var (
	// ErrHandoffUnavailable means this build cannot ask the daemon for a handoff.
	ErrHandoffUnavailable = errors.New("tymux: graceful handoff is not available in this build")
	// ErrHandoffRefused means the daemon declined, before changing anything (bad binary path,
	// failed preflight, schema range mismatch). It is still running and still serving.
	ErrHandoffRefused = errors.New("tymux: daemon refused the graceful handoff")
	// ErrHandoffVerifyFailed means the handoff was requested but the daemon did not come back as
	// expected. The daemon is never killed for it: panes may still be alive.
	ErrHandoffVerifyFailed = errors.New("tymux: graceful handoff could not be verified")
)

// handoffRequest mirrors tymux's PrepareHandoffRequest.
type handoffRequest struct {
	BinaryPath      string
	ExpectedVersion string
	DryRun          bool
}

// handoffReply is the daemon's answer to a PrepareHandoff call.
type handoffReply struct {
	Refused bool
	Reason  string
}

// prepareHandoffFn calls PrepareHandoff. The default fails closed until the pinned tymux client
// (TYMUX_VERSION) carries the RPC, so no build sends a handoff it cannot describe.
var prepareHandoffFn = func(context.Context, DaemonConfig, handoffRequest) (handoffReply, error) {
	return handoffReply{}, ErrHandoffUnavailable
}

var (
	handoffVerifyTimeout = 30 * time.Second
	handoffVerifyPoll    = 250 * time.Millisecond
)

// replaceViaHandoff swaps the running daemon for cfg.BinaryPath without ending any pane. The
// caller must have installed that binary by rename (never overwritten in place). Order: dry run
// (validation and preflight, changes nothing), the real request, then wait for the same pid to
// report the target version with the same live-session count. Every failure leaves the daemon
// alone: the old image resumes on a refusal and a rollback exec restores it on a failed adopt.
func replaceViaHandoff(ctx context.Context, cfg DaemonConfig, before DaemonVersionInfo, live int) (DaemonVersionInfo, error) {
	target := before.InstalledVersion
	req := handoffRequest{BinaryPath: cfg.BinaryPath, ExpectedVersion: target}

	dry := req
	dry.DryRun = true
	if err := requestHandoff(ctx, cfg, dry); err != nil {
		return before, err
	}
	if err := requestHandoff(ctx, cfg, req); err != nil {
		return before, err
	}

	deadline := time.Now().Add(handoffVerifyTimeout)
	for {
		after := DetectDaemonVersions(ctx, cfg)
		if after.Running && after.PID == before.PID && after.RunningVersion == target {
			n, err := liveSessionsFn(ctx, cfg)
			if err != nil || n != live {
				return after, fmt.Errorf("%w: %d live sessions before, %d after (count error: %v)", ErrHandoffVerifyFailed, live, n, err)
			}
			log.Info("[tymux] replaced tymuxd by graceful handoff", "from", before.RunningVersion, "to", target, "pid", after.PID, "live", live)
			return after, nil
		}
		if time.Now().After(deadline) {
			return after, fmt.Errorf("%w: pid %d did not report version %q within %s (daemon left running)", ErrHandoffVerifyFailed, before.PID, target, handoffVerifyTimeout)
		}
		select {
		case <-ctx.Done():
			return after, fmt.Errorf("%w: %v", ErrHandoffVerifyFailed, ctx.Err())
		case <-time.After(handoffVerifyPoll):
		}
	}
}

func requestHandoff(ctx context.Context, cfg DaemonConfig, req handoffRequest) error {
	reply, err := prepareHandoffFn(ctx, cfg, req)
	if err != nil {
		return fmt.Errorf("tymux: PrepareHandoff (dry_run=%t): %w", req.DryRun, err)
	}
	if reply.Refused {
		return fmt.Errorf("%w (dry_run=%t): %s", ErrHandoffRefused, req.DryRun, reply.Reason)
	}
	return nil
}
