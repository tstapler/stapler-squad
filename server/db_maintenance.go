package server

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/dbmaint"
)

// acAuditRetention is how long refused attempts, and the trail of deleted items, are kept.
const (
	acAuditRetention     = 90 * 24 * time.Hour
	acAuditPruneInterval = 6 * time.Hour
)

func (s *Server) startAcAuditPruning(ctx context.Context, storage interface {
	PruneAcAuditOlderThan(context.Context, time.Time) (int, error)
}) {
	s.goBackground(func() {
		t := time.NewTicker(acAuditPruneInterval)
		defer t.Stop()
		for {
			if n, err := storage.PruneAcAuditOlderThan(ctx, time.Now().Add(-acAuditRetention)); err != nil {
				log.Warn("ac audit prune failed", "err", err)
			} else if n > 0 {
				log.Info("pruned ac audit events", "count", n)
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	})
}

// startDBMaintenance reclaims free SQLite pages of the sessions, analytics and escape-events
// databases in the background until ctx is cancelled (see session/dbmaint).
func (s *Server) startDBMaintenance(ctx context.Context, deps *ServerDependencies) {
	if deps.Storage != nil {
		s.startAcAuditPruning(ctx, deps.Storage)
	}
	cfg := dbmaint.ConfigFromEnv()
	if cfg.Disabled {
		log.Info("db maintenance disabled", "env", dbmaint.EnvDisable)
		return
	}
	if deps.Storage != nil {
		if q := deps.Storage.MaintenanceDriver(); q != nil {
			m := dbmaint.New("sessions", q, cfg)
			s.goBackground(func() { m.Run(ctx) })
		}
	}
	if deps.AnalyticsDriver != nil {
		m := dbmaint.New("analytics", deps.AnalyticsDriver, cfg)
		s.goBackground(func() { m.Run(ctx) })
	}
	if deps.EscapeEventsDriver != nil {
		m := dbmaint.New("escape-events", deps.EscapeEventsDriver, cfg)
		s.goBackground(func() { m.Run(ctx) })
	}
}
