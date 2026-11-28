package server

import (
	"context"

	"github.com/tstapler/stapler-squad/server/prwatchhost"
)

// startPRWatch starts the PR watch inbox consumer (server/prwatchhost) on srv's
// background WaitGroup. Without a session service or storage nothing starts.
func startPRWatch(ctx context.Context, srv *Server, deps *ServerDependencies) {
	if deps.SessionService == nil || deps.Storage == nil {
		return
	}
	prwatchhost.Start(ctx, prwatchDeps(srv, deps))
}

// prwatchDeps names the concrete values the PR watch leaf runs on.
func prwatchDeps(srv *Server, deps *ServerDependencies) prwatchhost.Deps {
	return prwatchhost.Deps{
		Addr:       srv.GetAddr(),
		Sessions:   deps.SessionService,
		Store:      deps.Storage,
		Bus:        deps.EventBus,
		Go:         srv.goBackground,
		OnShutdown: func(hook func()) { srv.shutdownHooks = append(srv.shutdownHooks, hook) },
	}
}
