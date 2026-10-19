package server

import (
	"context"
	"net/http"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/wakes"
)

// registerWakeTriggerService builds the operator-facing wake service and
// registers its RPC handler. The dispatcher joins it later (startWakes calls
// SetBackend), so listing and cancelling work while the kill switch is off.
func registerWakeTriggerService(srv *Server, deps *ServerDependencies, guidance *services.GuidanceRequestService) *services.WakeTriggerService {
	titles := sessionTitles{svc: deps.SessionService, storage: deps.Storage}
	svcDeps := services.WakeTriggerDeps{
		Store: deps.Storage, Guidance: guidance, Titles: titles.Title,
		Enabled: func() bool { return wakeDispatcherEnabled(config.LoadConfig()) },
	}
	if deps.Storage != nil {
		notifier := wakeNotifier{bus: deps.EventBus}
		svcDeps.ExpireOverdue = func(ctx context.Context) error {
			_, err := wakes.ExpireOverdue(ctx, deps.Storage, notifier, time.Now())
			return err
		}
	}
	if deps.SessionService != nil {
		svcDeps.Owners = wakeOwners{svc: deps.SessionService, storage: deps.Storage}
	}
	svc := services.NewWakeTriggerService(svcDeps)
	path, handler := sessionv1connect.NewWakeTriggerServiceHandler(svc, ConnectOptions(deps.ErrorRegistry)...)
	apiPath := "/api" + path
	srv.RegisterConnectHandler(apiPath, http.StripPrefix("/api", handler))
	log.Info("Registered WakeTriggerService handler", "path", apiPath)
	return svc
}
