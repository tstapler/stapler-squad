package tymux

import (
	"context"
	"os"
	"strings"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"

	"github.com/tstapler/stapler-squad/log"
)

// restartModeEnv overrides the restart mode requested for new panes: never, on_failure (default)
// or always. An unrecognized value falls back to the default.
const restartModeEnv = "STAPLER_SQUAD_TYMUX_RESTART_MODE"

// restartPolicyFor returns the policy to send with CreateSession, or nil for "never" or when the
// daemon does not advertise pane.lifecycle (an older tymuxd would silently drop the field, leaving
// the caller believing panes restart). ON_FAILURE restarts a pane whose process dies non-zero or
// unknown, inside tymuxd's default budget (5 per 60s, then FAILED), so a clean exit still ends the
// session and a crash loop cannot spin forever.
func (s *tymuxGRPCSession) restartPolicyFor(ctx context.Context) *v1.RestartPolicy {
	mode := restartModeFromEnv()
	if mode == v1.RestartMode_RESTART_MODE_NEVER {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, lifecycleRPCTimeout)
	defer cancel()
	if !s.supportsPaneLifecycle(ctx) {
		log.Info("tymux: daemon lacks pane.lifecycle, creating session without a restart policy")
		return nil
	}
	return &v1.RestartPolicy{Mode: mode}
}

func restartModeFromEnv() v1.RestartMode {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(restartModeEnv))) {
	case "never":
		return v1.RestartMode_RESTART_MODE_NEVER
	case "always":
		return v1.RestartMode_RESTART_MODE_ALWAYS
	default:
		return v1.RestartMode_RESTART_MODE_ON_FAILURE
	}
}
