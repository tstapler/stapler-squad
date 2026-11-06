package server

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/session"
)

type prMaintenanceRecorder struct {
	calls   atomic.Int32
	storage atomic.Pointer[session.Storage]
	ctx     atomic.Value // context.Context
}

func (r *prMaintenanceRecorder) run(ctx context.Context, st *session.Storage) error {
	r.ctx.Store(&ctx)
	r.calls.Add(1)
	r.storage.Store(st)
	return nil
}

func buildCoreWithClient(t *testing.T, opts BuildOptions) *CoreDeps {
	t.Helper()
	envtest.NewIsolatedStateDir(t)
	opts.EntClient = session.NewTestEntRepository(t).GetEntClient()
	core, err := BuildCoreDepsWithOptions(opts)
	require.NoError(t, err)
	return core
}

// The service bootstrap runs PR maintenance exactly once, with the Storage that
// the rest of the dependencies are built on, inside BuildCoreDepsWithOptions and
// therefore before BuildServiceDeps constructs any poller, sweeper or ticker.
func TestBuildCoreDepsWithOptions_ShouldRunPRMaintenanceOnce_WhenRequested(t *testing.T) {
	rec := &prMaintenanceRecorder{}

	core := buildCoreWithClient(t, BuildOptions{RunPRMaintenance: true, PRMaintenanceRunner: rec.run})

	assert.Equal(t, int32(1), rec.calls.Load())
	assert.Same(t, core.Storage, rec.storage.Load())
	require.NotNil(t, core.StopPRMaintenance)
	core.StopPRMaintenance() // idempotent, safe with no ticker running
	core.StopPRMaintenance()
}

// The caller's context reaches the maintenance run, so SIGTERM during boot cancels
// its retries.
func TestBuildCoreDepsWithOptions_ShouldPassContextToMaintenance(t *testing.T) {
	rec := &prMaintenanceRecorder{}
	ctx, cancel := context.WithCancel(context.Background())

	buildCoreWithClient(t, BuildOptions{RunPRMaintenance: true, PRMaintenanceRunner: rec.run, Context: ctx})
	got := *rec.ctx.Load().(*context.Context)
	require.NoError(t, got.Err())
	cancel()
	assert.Error(t, got.Err(), "cancelling the boot context cancels the maintenance run")
}

// buildMCPDeps uses the plain BuildCoreDeps(): no maintenance, no ticker.
func TestBuildCoreDepsWithOptions_ShouldNotRunPRMaintenance_WhenNotRequested(t *testing.T) {
	rec := &prMaintenanceRecorder{}

	core := buildCoreWithClient(t, BuildOptions{PRMaintenanceRunner: rec.run})

	assert.Equal(t, int32(0), rec.calls.Load())
	assert.Nil(t, core.StopPRMaintenance)
}

func TestToServerDeps_ShouldCarryStopPRMaintenance(t *testing.T) {
	var stopped atomic.Bool
	rt := &RuntimeDeps{ServiceDeps: &ServiceDeps{CoreDeps: &CoreDeps{StopPRMaintenance: func() { stopped.Store(true) }}}}

	deps := rt.ToServerDeps()

	require.NotNil(t, deps.StopPRMaintenance)
	deps.StopPRMaintenance()
	assert.True(t, stopped.Load())
}
