package services

import (
	"context"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session"
)

// terminalWriteLeaseFlagName is the feature flag behind the per-instance
// terminal write lease (plan Story 5.0). Global only, default on, no env var.
// Off makes every lease non-exclusive (the pre-lease behavior), so a lease
// regression is reversible without a deploy.
const terminalWriteLeaseFlagName = "terminal_write_lease"

// terminalWriteLeaseAuditPolicy declares the off flip an escape hatch: it
// loosens (it removes serialization) but it is persisted first and recorded
// with only the fallback log record when the audit sink is down, never refused
// with Internal, so a sink outage cannot block reversing a lease regression.
// The on flip is a tightening flip (queued line).
var terminalWriteLeaseAuditPolicy = FlagAuditPolicy{
	Loosening:   func(enabled bool) bool { return !enabled },
	EscapeHatch: true,
}

// leaseFlagController applies the persisted flag to the injected atomic every
// instance reads. Its IsEnabled is the live value GetFeatureFlags reports.
type leaseFlagController struct{ flag *session.LeaseFlag }

func (c leaseFlagController) Enable(context.Context) error { c.flag.SetEnabled(true); return nil }
func (c leaseFlagController) Disable() error               { c.flag.SetEnabled(false); return nil }
func (c leaseFlagController) IsEnabled() bool              { return c.flag.Enabled() }

// wireLeaseFlag connects terminal_write_lease to session.DefaultLeaseFlag: the
// controller applies every flip at once, and the starting value is the
// persisted one. An explicit persisted false is the only way to start with the
// lease off; a missing or unreadable config keeps it on (fails closed).
func (s *SessionService) wireLeaseFlag() {
	flag := session.DefaultLeaseFlag
	flag.SetEnabled(config.LoadConfig().GetFeatureFlagWithDefault(terminalWriteLeaseFlagName, true))
	s.featureFlagSvc.SetFeatureController(terminalWriteLeaseFlagName, leaseFlagController{flag: flag})
}
