package services

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// hiddenSessionReadonlyGuardsFlagName is the live escape hatch for the unary
// write guards (plan Story 5.2). Global only, default on, no env var. Off lets
// WriteToSession, the steer branch, RestartSession/RestartShell, SpawnShell, SwitchWorkspace
// and UpdateSession's program/auto_approve accept a hidden target as on main;
// the stream drops and Reply are governed elsewhere.
const hiddenSessionReadonlyGuardsFlagName = "hidden_session_readonly_guards"

// guardsFlagAuditPolicy: off loosens, so it is audited durably before it is
// persisted; it is also the hatch for a faulty sink, so it is never refused
// when the sink is down (fallback record only). On is a tightening flip.
var guardsFlagAuditPolicy = FlagAuditPolicy{
	Loosening:   func(enabled bool) bool { return !enabled },
	EscapeHatch: true,
}

// guardsFlagController applies the persisted flag to the injected atomic the
// unary handlers read.
type guardsFlagController struct{ flag *UnaryGuardsFlag }

func (c guardsFlagController) Enable(context.Context) error { c.flag.SetEnabled(true); return nil }
func (c guardsFlagController) Disable() error               { c.flag.SetEnabled(false); return nil }
func (c guardsFlagController) IsEnabled() bool              { return c.flag.GuardsEnabled() }

// guardsOffStatusDetail is the Settings status line while the guards are off.
func (s *SessionService) guardsOffStatusDetail() string {
	if s.guards.GuardsEnabled() {
		return ""
	}
	return fmt.Sprintf("Hidden-session write guards are OFF: %d writes bypassed", s.guardBypass.count.Load())
}

// wireGuardsFlag connects the flag to the service's atomic: the controller
// applies every flip at once and the starting value is the persisted one. Only
// an explicit persisted false starts with the guards off; a missing or
// unreadable config keeps them on.
func (s *SessionService) wireGuardsFlag() {
	s.guards.SetEnabled(config.LoadConfig().GetFeatureFlagWithDefault(hiddenSessionReadonlyGuardsFlagName, true))
	ff := s.featureFlagSvc
	ff.SetFeatureController(hiddenSessionReadonlyGuardsFlagName, guardsFlagController{flag: s.guards})
	ff.AddStatusDetailSource(hiddenSessionReadonlyGuardsFlagName, s.guardsOffStatusDetail)
}

// guardBypassState counts steers allowed by the off flag and rate-limits the
// WARN to once a minute.
type guardBypassState struct {
	count atomic.Uint64
	mu    sync.Mutex
	last  time.Time
	now   func() time.Time // injected clock; nil means time.Now
}

const guardBypassWarnEvery = time.Minute

// note counts one bypassed write and reports whether a WARN is due.
func (g *guardBypassState) note() (warn bool) {
	g.count.Add(1)
	now := time.Now
	if g.now != nil {
		now = g.now
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if t := now(); g.last.IsZero() || t.Sub(g.last) >= guardBypassWarnEvery {
		g.last = t
		return true
	}
	return false
}

// auditGuardBypass writes the blocking, fsynced guard_bypass line before a
// steer to a non-qualifying hidden target goes through (guards off). With the
// sink down the write is refused: the flag exists to restore the review
// composer, not to open writes to triage, diagnose and other sessions blind.
func (s *SessionService) auditGuardBypass(ctx context.Context, d steerDecision, inst *session.Instance, msg string) (AuditLine, error) {
	line := s.steerAuditLine(withSteerRequestFacts(ctx, d.facts), inst, msg)
	line.Kind = auditKindGuardBypass
	line.Phase = auditPhaseRequest
	sink := s.auditSink()
	var err error
	if sink == nil {
		err = ErrAuditFailed
	} else {
		err = sink.AppendBounded(ctx, line)
	}
	if err != nil {
		s.countBacklogSteer(steerOutcomeAuditFailed)
		log.Error("[GuardBypass] audit append failed; steer refused", "session", line.SessionUUID, "err", err)
		return line, errSteerAuditUnavailable
	}
	s.countBacklogSteer(steerOutcomeGuardBypass)
	if s.guardBypass.note() {
		log.Warn("[GuardBypass] steer to a hidden session allowed: hidden_session_readonly_guards is off",
			"session", line.SessionUUID, "kind", line.Kind)
	}
	return line, nil
}
