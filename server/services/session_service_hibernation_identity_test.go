package services

import (
	"path/filepath"
	"testing"
	"testing/synctest"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/queue"
)

type hibernationIdentityStore struct {
	session.InstanceStore
	loaded *instance.Instance
	saved  *instance.Instance
}

func (s *hibernationIdentityStore) LoadInstances() ([]*instance.Instance, error) {
	return []*instance.Instance{s.loaded}, nil
}

func (s *hibernationIdentityStore) SaveInstances(instances []*instance.Instance) error {
	s.saved = instances[0]
	return nil
}

func newHibernationIdentityFixture(t *testing.T) (*SessionService, *instance.Instance, *hibernationIdentityStore) {
	t.Helper()
	canonical, err := instance.NewInstance(instance.InstanceOptions{
		Title: "hibernate-identity", Path: filepath.Join(t.TempDir(), "not-created"), Program: "bash",
	})
	require.NoError(t, err)
	canonical.ForceStatus(domain.Active)
	stale := &instance.Instance{Title: canonical.Title, UUID: canonical.UUID, Status: domain.Stopped}
	store := &hibernationIdentityStore{loaded: stale}
	bus := events.NewEventBus(8)
	t.Cleanup(bus.Close)
	registry := session.NewRegistry(nil, nil)
	t.Cleanup(registry.Shutdown)
	poller := session.NewReviewQueuePoller(queue.NewReviewQueue(), instance.NewInstanceStatusManager(), nil)
	poller.AddInstance(canonical)
	svc := &SessionService{
		storage: store, eventBus: bus, registry: registry, ReviewQueuePoller: poller,
		AutonomousSvc: &AutonomousOrchestrationService{},
	}
	release := instance.HoldHibernationTeardownForTest(t, canonical)
	t.Cleanup(func() {
		canonical.ForceStatus(domain.Stopped)
		instance.StopSessionDriver(canonical)
		release()
		instance.JoinHibernation(canonical)
	})
	return svc, canonical, store
}

func TestHibernateArchiveResumeRPC_StaysArchived(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc, canonical, store := newHibernationIdentityFixture(t)
		_, err := svc.HibernateSession(t.Context(), connect.NewRequest(&sessionv1.HibernateSessionRequest{Id: canonical.UUID}))
		require.NoError(t, err)
		require.Nil(t, svc.FindLiveInstance(canonical.UUID))
		require.NoError(t, svc.ArchiveSessionByUUID(t.Context(), canonical.UUID))
		require.True(t, canonical.IsArchived())
		require.Equal(t, domain.Stopped, canonical.GetLifecycleStatus())
		require.Same(t, canonical, store.saved)
		_, err = svc.ResumeHibernatedSession(t.Context(), connect.NewRequest(&sessionv1.ResumeHibernatedSessionRequest{Id: canonical.UUID}))
		require.Error(t, err)
		require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
		synctest.Wait()
		require.True(t, canonical.IsArchived())
		require.Equal(t, domain.Stopped, canonical.GetLifecycleStatus())
		require.False(t, canonical.Started())
	})
}

func TestHibernateResumeRPC_RetainsCanonicalInstance(t *testing.T) {
	for _, name := range []string{"poller only", "already registered", "registry beats stale poller"} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				svc, canonical, store := newHibernationIdentityFixture(t)
				if name != "poller only" {
					_, err := svc.registry.Register(instance.NewLiveInstance(canonical))
					require.NoError(t, err)
				}
				if name == "registry beats stale poller" {
					svc.ReviewQueuePoller.RemoveInstance(canonical.Title)
					svc.ReviewQueuePoller.AddInstance(store.loaded)
				}
				_, err := svc.HibernateSession(t.Context(), connect.NewRequest(&sessionv1.HibernateSessionRequest{Id: canonical.UUID}))
				require.NoError(t, err)
				require.Nil(t, svc.FindLiveInstance(canonical.UUID), "manual hibernation must still remove poller membership")
				require.Same(t, canonical, store.saved)
				require.True(t, canonical == svc.registry.Lookup(canonical.UUID).Instance)

				_, err = svc.ResumeHibernatedSession(t.Context(), connect.NewRequest(&sessionv1.ResumeHibernatedSessionRequest{Id: canonical.UUID}))
				require.NoError(t, err)
				require.Same(t, canonical, store.saved, "resume must keep the original teardown wait group")
				synctest.Wait()
				require.Equal(t, domain.Active, canonical.Snapshot().Status, "resume must remain blocked behind the held teardown")
				require.False(t, canonical.Started())
			})
		})
	}
}
