package server

import (
	"context"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/dbmaint"
)

// startDBMaintenance reclaims free SQLite pages of the sessions and analytics
// databases in the background until ctx is cancelled (see session/dbmaint).
func (s *Server) startDBMaintenance(ctx context.Context, deps *ServerDependencies) {
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
}
