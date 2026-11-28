package server

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/backlogsvc"
	"github.com/tstapler/stapler-squad/server/services/julesrpc"
	"github.com/tstapler/stapler-squad/session"
)

func newBuildJulesDeps(t *testing.T) *ServiceDeps {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	envtest.NewIsolatedStateDir(t)
	storage, err := session.NewStorageWithRepository(session.NewTestEntRepository(t))
	require.NoError(t, err)
	bus := events.NewEventBus(8)
	t.Cleanup(bus.Close)
	svc := services.NewSessionService(storage, bus)
	t.Cleanup(svc.Shutdown)
	return &ServiceDeps{CoreDeps: &CoreDeps{Storage: storage, SessionService: svc}}
}

// privateField reads an unexported field, so the wiring can be checked without
// adding production accessors.
func privateField(t *testing.T, v reflect.Value, name string) reflect.Value {
	t.Helper()
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		require.False(t, v.IsNil(), "nil value holding field %s", name)
		v = v.Elem()
	}
	f := v.FieldByName(name)
	require.True(t, f.IsValid(), "%s has no field %s", v.Type(), name)
	return f
}

// counterAddress requires the usage field to hold a real counter and returns its identity.
func counterAddress(t *testing.T, usage reflect.Value) uintptr {
	t.Helper()
	require.False(t, usage.IsNil(), "no usage counter installed")
	require.Equal(t, reflect.TypeOf(&julesrpc.JulesUsageCounter{}), usage.Elem().Type())
	require.False(t, usage.Elem().IsNil(), "nil usage counter installed")
	return usage.Elem().Pointer()
}

// configServiceCounter is the counter GetJulesConfig reports from.
func configServiceCounter(t *testing.T, svc *services.SessionService) uintptr {
	t.Helper()
	configSvc := privateField(t, reflect.ValueOf(svc), "julesConfigSvc")
	return counterAddress(t, privateField(t, configSvc, "usage"))
}

func TestBuildJules_should_ReportFromARealUsageCounter_When_JulesIsDisabled(t *testing.T) {
	deps := newBuildJulesDeps(t)

	buildJules(deps, &config.Config{}, nil)

	configServiceCounter(t, deps.SessionService)
}

func TestBuildJules_should_ShareOneUsageCounter_When_JulesIsEnabled(t *testing.T) {
	deps := newBuildJulesDeps(t)
	keyring.MockInit()
	require.NoError(t, keyring.Set("stapler-squad-jules", "api-key", "test-key"))
	cfg := &config.Config{}
	cfg.Jules.Enabled = true
	backlog := &backlogsvc.BacklogService{}

	poller := buildJules(deps, cfg, backlog)

	require.NotNil(t, poller, "enabled with a resolvable key must build the poller")
	reported := configServiceCounter(t, deps.SessionService)
	dispatcher := privateField(t, reflect.ValueOf(backlog), "julesDispatcher")
	require.Equal(t, reported, counterAddress(t, privateField(t, dispatcher, "usage")), "dispatcher counts elsewhere")
	require.Equal(t, reported, counterAddress(t, privateField(t, reflect.ValueOf(poller), "usage")), "poller counts elsewhere")
}
