package server

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/log"
	servermcp "github.com/tstapler/stapler-squad/server/mcp"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/guidance"
	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session/autonomous"
	"github.com/tstapler/stapler-squad/session/git"
)

// registerWakeTriggerService builds the operator-facing wake service and
// registers its RPC handler. The dispatcher joins it later (startWakes calls
// SetBackend), so listing and cancelling work while the kill switch is off.
func registerWakeTriggerService(srv *Server, deps *ServerDependencies, guidanceSvc *guidance.GuidanceRequestService) *services.WakeTriggerService {
	titles := sessionTitles{svc: deps.SessionService, storage: deps.Storage}
	svcDeps := services.WakeTriggerDeps{
		Store: deps.Storage, Guidance: guidanceSvc, Titles: titles.Title,
		Enabled: func() bool { return wakeDispatcherEnabled(config.LoadConfig()) },
	}
	deps.sharedWakeMetrics()
	if deps.Storage != nil {
		notifier := wakeNotifier{bus: deps.EventBus}
		svcDeps.ExpireOverdue = func(ctx context.Context) error {
			_, err := wakes.ExpireOverdue(ctx, deps.Storage, notifier, deps.sharedWakeMetrics(), time.Now())
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

// sharedWakeMetrics returns the process's one wake counter mirror, creating it
// on first use. The wake service, the dispatcher and the MCP tools all read it.
func (d *ServerDependencies) sharedWakeMetrics() *wakes.Metrics {
	d.wakeMetricsMu.Lock()
	defer d.wakeMetricsMu.Unlock()
	if d.WakeMetrics == nil {
		d.WakeMetrics = wakes.NewMetrics()
	}
	return d.WakeMetrics
}

// mcpCoreOptions is the HTTP MCP core's whole CoreOption set.
func mcpCoreOptions(deps *ServerDependencies) []servermcp.CoreOption {
	return slices.Concat(flowMCPOptions(deps), []servermcp.CoreOption{wakeMCPOptions(deps), servermcp.WithWorktreeOpener(NewMCPWorktreeOpener()), servermcp.WithGitHub(NewMCPGitHub()), servermcp.WithUserPRCache(mcpUserPRSource(deps.UserPRCache)), servermcp.WithNudgeGate(mcpNudgeGate(autonomous.VerifyNudgeSafeToWrite))})
}

// The real worktree must keep satisfying the MCP tools' port.
var _ servermcp.Worktree = (*git.GitWorktree)(nil)

// NewMCPWorktreeOpener is the production WorktreeOpener the HTTP and stdio MCP
// cores share. It returns a nil interface, not a typed-nil, for unusable paths.
func NewMCPWorktreeOpener() servermcp.WorktreeOpener {
	return func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) servermcp.Worktree {
		wt := git.NewGitWorktreeFromStorage(repoPath, worktreePath, sessionName, branchName, baseCommitSHA)
		if wt == nil {
			return nil
		}
		return wt
	}
}

// wakeMCPOptions hands the shared wake metrics to the HTTP MCP core.
func wakeMCPOptions(deps *ServerDependencies) servermcp.CoreOption {
	return servermcp.WithWakeMetrics(deps.sharedWakeMetrics())
}
