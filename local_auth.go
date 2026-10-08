package main

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/localtoken"
	"github.com/tstapler/stapler-squad/server"
	serverauth "github.com/tstapler/stapler-squad/server/auth"
	"github.com/tstapler/stapler-squad/server/middleware"
)

//nolint:gochecknoglobals // process-wide singleton: local and remote listeners must share one session store
var (
	authSessionsOnce sync.Once
	authSessions     *serverauth.SessionManager
)

// sharedAuthSessions returns the process-wide auth session manager. The local
// (require_local_auth) and remote listeners must share one: two managers on the
// same auth-sessions.json would overwrite each other's persisted sessions.
func sharedAuthSessions(configDir string) *serverauth.SessionManager {
	authSessionsOnce.Do(func() {
		authSessions = serverauth.NewSessionManager(filepath.Join(configDir, "auth-sessions.json"))
	})
	return authSessions
}

// setupLocalAuth enforces credentials on the :8543 listener when
// cfg.RequireLocalAuth is set: passkey session cookie or the local API token as
// a Bearer, plus the one-time-code login routes used by --open-url. No-op when
// the flag is off.
func setupLocalAuth(srv *server.Server, cfg *config.Config) error {
	if !cfg.RequireLocalAuth {
		return nil
	}
	configDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("local auth: get config dir: %w", err)
	}
	token, err := localtoken.LoadOrCreate(localtoken.Path(configDir))
	if err != nil {
		return fmt.Errorf("local auth: %w", err)
	}
	sessions := sharedAuthSessions(configDir)
	validator := serverauth.NewLocalValidator(sessions, token)
	serverauth.RegisterLocalLoginRoutes(srv.Mux(), serverauth.NewLocalLogin(sessions, validator))
	srv.SetupAuth(middleware.Auth(validator))
	log.Info("local listener auth enabled", "token_file", localtoken.Path(configDir))
	return nil
}
