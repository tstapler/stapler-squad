package services

import (
	"context"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/session/instance"
)

// terminalWriteLeaseAuditPolicy declares the off flip an escape hatch: it
// loosens (it removes serialization) but it is persisted first and recorded
// with only the fallback log record when the audit sink is down, never refused
// with Internal, so a sink outage cannot block reversing a lease regression.
// The on flip is a tightening flip (queued line).
var terminalWriteLeaseAuditPolicy = featureflags.FlagAuditPolicy{
	Loosening:   func(enabled bool) bool { return !enabled },
	EscapeHatch: true,
}

// leaseFlagController applies the persisted flag to the injected atomic every
// instance reads. Its IsEnabled is the live value GetFeatureFlags reports.
type leaseFlagController struct{ flag *instance.LeaseFlag }

func (c leaseFlagController) Enable(context.Context) error { c.flag.SetEnabled(true); return nil }
func (c leaseFlagController) Disable() error               { c.flag.SetEnabled(false); return nil }
func (c leaseFlagController) IsEnabled() bool              { return c.flag.Enabled() }

// wireLeaseFlag connects terminal_write_lease to session.DefaultLeaseFlag: the
// controller applies every flip at once, and the starting value is the
// persisted one. An explicit persisted false is the only way to start with the
// lease off; a missing or unreadable config keeps it on (fails closed).
func (s *SessionService) wireLeaseFlag() {
	flag := instance.DefaultLeaseFlag
	flag.SetEnabled(config.LoadConfig().GetFeatureFlagWithDefault(featureflags.TerminalWriteLeaseFlagName, true))
	s.featureFlagSvc.SetFeatureController(featureflags.TerminalWriteLeaseFlagName, leaseFlagController{flag: flag})
}
