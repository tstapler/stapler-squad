package services

import (
	"context"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/server/services/sessioncreate"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/backlog"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/queue"
	"github.com/tstapler/stapler-squad/session/registry"
	"github.com/tstapler/stapler-squad/session/sshremote"
	"github.com/tstapler/stapler-squad/session/summaries"
	"github.com/tstapler/stapler-squad/session/tagpoller"
)

type wiringPermissionHandler struct {
	sshremote.PermissionRequestHandler
}

// Deps are taken before anything is installed, so each getter has to read the
// field when it is called, not when it is built.
func TestCreateDeps_ShouldReadTheServiceFieldsAtCallTime_When_SettersRunLater(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	bus := events.NewEventBus(8)
	t.Cleanup(bus.Close)
	svc := NewSessionService(storage, bus)
	t.Cleanup(func() { svc.Shutdown() })
	deps := svc.createDeps()

	v := reflect.ValueOf(deps)
	for i := 0; i < v.NumField(); i++ {
		assert.False(t, v.Field(i).IsZero(), "Deps.%s is not wired", v.Type().Field(i).Name)
	}

	assert.Same(t, bus, deps.Bus)
	assert.Equal(t, session.InstanceStore(storage), deps.Store)
	assert.Same(t, svc.promptStore, deps.PromptStore)
	assert.Equal(t, svc.testTmuxServerSocket, deps.TmuxServerSocket)
	assert.Same(t, storage, deps.ConcreteStorage())
	assert.Same(t, storage, deps.ConcStorage())

	// Nothing optional is installed yet: each must read as nil, not as a typed nil.
	assert.Nil(t, deps.Registry())
	assert.Nil(t, deps.Poller())
	assert.Nil(t, deps.TagPoller())
	assert.Nil(t, deps.StatusManager())
	assert.Nil(t, deps.UserPRCache())
	keys, hosts := deps.RemoteStores()
	assert.Nil(t, keys)
	assert.Nil(t, hosts)
	assert.True(t, deps.PermissionHandler() == nil, "an unset handler must compare equal to nil")
	assert.Nil(t, deps.BacklogListener())
	assert.Nil(t, deps.SummaryGenerator())
	assert.Nil(t, deps.HeadlessPool())
	assert.Nil(t, deps.PhaseHook())

	reg := &registry.Registry{}
	poller := session.NewReviewQueuePoller(queue.NewReviewQueue(), instance.NewInstanceStatusManager(), nil)
	tags := &tagpoller.SessionTagClassificationPoller{}
	statusMgr := instance.NewInstanceStatusManager()
	prCache := &githubpkg.UserPRCache{}
	keyStore := sshremote.NewKeyStore()
	knownHosts := &sshremote.KnownHostsStore{}
	handler := &wiringPermissionHandler{}
	listener := &backlog.BacklogLifecycleListener{}
	summary := &summaries.SessionSummaryGenerator{}
	pool := headless.NewPoolWithRunner(headless.PoolConfig{}, wiringRunner{})
	phases := 0

	svc.SetRegistry(reg)
	svc.SetReviewQueuePoller(poller)
	svc.sessionTagPoller = tags
	svc.SetStatusManager(statusMgr)
	svc.userPRCache = prCache
	svc.SetRemoteDeps(keyStore, knownHosts)
	svc.SetPermissionRequestHandler(handler)
	svc.backlogLifecycleListener = listener
	svc.sessionSummaryGenerator = summary
	svc.headlessPool = pool
	svc.creationPhaseHook = func(string) { phases++ }
	svc.SetMCPServerURL(func() string { return "http://late" })

	assert.Same(t, reg, deps.Registry())
	assert.Same(t, poller, deps.Poller())
	assert.Same(t, tags, deps.TagPoller())
	assert.Same(t, statusMgr, deps.StatusManager())
	assert.Same(t, prCache, deps.UserPRCache())
	keys, hosts = deps.RemoteStores()
	assert.Same(t, keyStore, keys)
	assert.Same(t, knownHosts, hosts)
	assert.Same(t, handler, deps.PermissionHandler())
	assert.Same(t, listener, deps.BacklogListener())
	assert.Same(t, summary, deps.SummaryGenerator())
	assert.Same(t, pool, deps.HeadlessPool())
	deps.PhaseHook()("x")
	assert.Equal(t, 1, phases)
	assert.Equal(t, "http://late", deps.MCPServerURL())

	assert.Same(t, svc.AutonomousSvc, deps.Autonomy())
	assert.NotNil(t, deps.DriverClient())
	assert.Same(t, svc.sshClientPool(), deps.SSHClientPool())
	assert.Equal(t, svc.gitBackendOrDefault(), deps.GitBackend())
	assert.Equal(t, sessioncreate.DefaultResolutionTimeout, deps.ResolutionTimeout())
	svc.creationResolutionTimeout = time.Second
	assert.Equal(t, time.Second, deps.ResolutionTimeout())
	resolved := false
	svc.githubResolver = func(context.Context, string, []string) (string, *instance.GitHubRef, error) {
		resolved = true
		return "", nil, nil
	}
	_, _, err := deps.GitHubResolver()(context.Background(), "", nil)
	require.NoError(t, err)
	assert.True(t, resolved)
}

// Method values carry the method name, so a field wired to the wrong method fails.
func TestCreateDeps_ShouldCallTheNamedServiceMethods(t *testing.T) {
	t.Parallel()
	svc := &SessionService{}
	deps := svc.createDeps()
	for method, fn := range map[string]any{
		"GetStorage":              deps.ConcreteStorage,
		"sshClientPool":           deps.SSHClientPool,
		"gitBackendOrDefault":     deps.GitBackend,
		"resolveMCPServerURL":     deps.MCPServerURL,
		"workflowNames":           deps.WorkflowNames,
		"workspacePeersBlockFor":  deps.WorkspacePeersBlock,
		"trackCleanup":            deps.Track,
		"wireCallbacks":           deps.WireCallbacks,
		"indexSessionForDelivery": deps.IndexForDelivery,
		"FindLiveInstance":        deps.FindLive,
		"GetInstances":            deps.Instances,
		"removeFromAllPollers":    deps.RemoveFromAllPollers,
		"ArchiveSession":          deps.ArchiveSession,
	} {
		name := runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
		assert.True(t, strings.HasSuffix(name, ".(*SessionService)."+method+"-fm"), "%s is wired to %s", method, name)
	}
}
