package tymux

import (
	"context"
	"errors"
	"fmt"
	"slices"
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

// readHandoffGenerationFn reads ServerInfo.handoff_generation. ok is false while the pinned
// tymux client or the daemon does not expose it; then only a version change can prove a handoff,
// and a same-version handoff stays fail-closed.
var readHandoffGenerationFn = func(context.Context, DaemonConfig) (gen uint64, ok bool, err error) {
	return 0, false, nil
}

var (
	handoffVerifyTimeout = 30 * time.Second       //nolint:novartestseam // same package-var seam as daemonStopGracePeriod; threading a duration through ReplaceDaemon's signature would touch every caller
	handoffVerifyPoll    = 250 * time.Millisecond //nolint:novartestseam // see handoffVerifyTimeout
)

// replaceViaHandoff swaps the running daemon for cfg.BinaryPath without ending any pane. The
// caller must have installed that binary by rename (never overwritten in place). Order: dry run
// (validation and preflight, changes nothing), the real request, then wait until a successful
// GetServerInfo shows the same pid at the target version with no fewer live sessions. Per the
// tymux design the old image resumes on a refusal and a rollback exec restores it on a failed
// adopt; squad itself never signals the daemon here.
//
// A refusal or an unavailable adapter happens before anything changed. An error from the real
// request is ambiguous (the daemon freezes and execs, so the connection can drop mid-call) and
// goes to verification, never straight to a fallback.
func replaceViaHandoff(ctx context.Context, cfg DaemonConfig, before DaemonVersionInfo, live int) (DaemonVersionInfo, error) {
	target := before.InstalledVersion
	gen0, hasGen, err := readHandoffGenerationFn(ctx, cfg)
	if err != nil {
		return before, fmt.Errorf("%w: reading handoff_generation: %v", ErrHandoffRefused, err)
	}
	if !hasGen && (target == "" || before.Direction == VersionSame) {
		// Without handoff_generation, the same pid at the same version cannot show a handoff
		// happened, and an unknown target cannot be verified at all.
		return before, fmt.Errorf("%w: target version %q is unknown or equals the running version and the daemon exposes no handoff_generation, so completion cannot be verified", ErrHandoffRefused, target)
	}
	req := handoffRequest{BinaryPath: cfg.BinaryPath, ExpectedVersion: target}

	dry := req
	dry.DryRun = true
	if err := requestHandoff(ctx, cfg, dry); err != nil {
		return before, err
	}
	if err := requestHandoff(ctx, cfg, req); err != nil {
		if errors.Is(err, ErrHandoffRefused) || errors.Is(err, ErrHandoffUnavailable) {
			return before, err
		}
		log.Warn("[tymux] PrepareHandoff errored mid-request; verifying instead of assuming failure", "err", err)
	}
	return verifyHandoff(ctx, cfg, before, target, live, gen0, hasGen)
}

// verifyHandoff polls until the daemon at before.PID answers GetServerInfo at the target version
// and still has at least live sessions. It asks GetServerInfo directly: the process-probe
// fallback in DetectDaemonVersions would read the already-renamed binary's version and could pass
// before the exec. More live sessions than before is fine (squad may have created one).
func verifyHandoff(ctx context.Context, cfg DaemonConfig, before DaemonVersionInfo, target string, live int, gen0 uint64, hasGen bool) (DaemonVersionInfo, error) {
	deadline := time.Now().Add(handoffVerifyTimeout)
	lastSeen := "no GetServerInfo answer"
	for {
		if si, err := serverInfoFn(ctx, cfg); err == nil {
			lastSeen = fmt.Sprintf("pid %d version %q", si.GetPid(), si.GetVersion())
			if int(si.GetPid()) == before.PID && handoffObserved(ctx, cfg, si.GetVersion(), target, gen0, hasGen) {
				n, lerr := liveSessionsFn(ctx, cfg)
				switch {
				case lerr != nil:
					lastSeen += fmt.Sprintf(", live count error: %v", lerr)
				case n < live:
					lastSeen += fmt.Sprintf(", %d live sessions (was %d)", n, live)
				default:
					log.Info("[tymux] replaced tymuxd by graceful handoff", "from", before.RunningVersion, "to", target, "pid", before.PID, "live", n)
					after := before
					after.RunningVersion, after.ProtocolVersion = si.GetVersion(), si.GetProtocolVersion()
					after.Direction = VersionSame
					after.GracefulHandoff = slices.Contains(si.GetCapabilities(), capGracefulHandoff)
					return after, nil
				}
			}
		}
		if time.Now().After(deadline) {
			return before, fmt.Errorf("%w: wanted pid %d at version %q with >= %d live sessions within %s, last saw %s (daemon left running)", ErrHandoffVerifyFailed, before.PID, target, live, handoffVerifyTimeout, lastSeen)
		}
		select {
		case <-ctx.Done():
			return before, fmt.Errorf("%w: %v", ErrHandoffVerifyFailed, ctx.Err())
		case <-time.After(handoffVerifyPoll):
		}
	}
}

// handoffObserved: with handoff_generation the counter must have advanced (and the version match
// when a target is known); without it the version must equal the target.
func handoffObserved(ctx context.Context, cfg DaemonConfig, version, target string, gen0 uint64, hasGen bool) bool {
	versionOK := target == "" || CompareVersions(version, target) == VersionSame
	if !hasGen {
		return versionOK
	}
	gen, ok, err := readHandoffGenerationFn(ctx, cfg)
	return err == nil && ok && gen > gen0 && versionOK
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
