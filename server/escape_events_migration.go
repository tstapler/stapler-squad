package server

import (
	"context"
	"path/filepath"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/analytics/escapedb"
)

// escapeEventsMigrationDelay keeps the copy off the startup path, when the
// process is busiest.
const escapeEventsMigrationDelay = time.Minute

// startEscapeEventsMigration copies the legacy analytics.db escape_events rows into
// db/escape_events.db in the background (see escapedb.MigrateFromAnalyticsDB).
// Flag escape_events_db_migrate (default on) is read once here. Once the copy is
// done, STAPLER_SQUAD_ESCAPE_LEGACY_DROP (default off) may drop the legacy table.
func (s *Server) startEscapeEventsMigration(ctx context.Context, deps *ServerDependencies, cfg *config.Config) {
	if deps.EscapeEventsDB == nil {
		return
	}
	if !cfg.EscapeEventsDBMigrateEnabled() {
		log.Info("escape events migration disabled", "flag", config.EscapeEventsDBMigrateFeatureFlag)
		return
	}
	configDir, err := config.GetConfigDir()
	if err != nil {
		log.Warn("escape events migration skipped: no config dir", "err", err)
		return
	}
	dropMode := escapedb.LegacyDropModeFromEnv()
	if dropMode == escapedb.LegacyDropOff {
		log.Info("legacy escape_events drop is off; set " + escapedb.LegacyDropEnv + "=dry-run or on to enable")
	}
	s.goBackground(func() {
		timer := time.NewTimer(escapeEventsMigrationDelay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		st, err := escapedb.MigrateFromAnalyticsDB(ctx, deps.EscapeEventsDB.Writer(),
			filepath.Join(configDir, "analytics.db"), escapedb.MigrationConfig{})
		switch {
		case ctx.Err() != nil:
			log.Info("escape events migration paused for shutdown; resumes next start", "scanned", st.Scanned)
		case err != nil:
			log.Warn("escape events migration failed; resumes next start", "err", err, "scanned", st.Scanned, "phase", st.Phase)
		case st.Done() && dropMode != escapedb.LegacyDropOff && deps.AnalyticsDriver != nil:
			res, dropErr := escapedb.DropLegacyEscapeEvents(ctx, deps.AnalyticsDriver,
				filepath.Join(configDir, "analytics.db"), escapedb.Path(configDir), escapedb.LegacyDropConfig{Mode: dropMode, RetentionDays: cfg.EscapeAnalyticsRetentionDays})
			if dropErr != nil && ctx.Err() == nil {
				log.Warn("legacy escape_events drop failed; resumes next start", "err", dropErr, "outcome", res.Outcome, "deleted", res.Deleted)
			}
		}
	})
}
