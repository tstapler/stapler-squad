package svctest

import (
	"context"
	"sync"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	gogitconfig "github.com/go-git/go-git/v5/config"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/jules"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// newTestJulesRepoWithRemote creates a real (empty) git repository at a
// fresh temp directory with an "origin" remote pointing at remoteURL, so
// resolveJulesOwnerRepo(dir) resolves exactly as it would against a real
// worktree — mirroring session.newTestRepoWithRemote
// (session/backlog_lifecycle_pr_trigger_test.go), which this package cannot
// import directly (unexported, different package).
func NewTestJulesRepoWithRemote(t *testing.T, remoteURL string) string {
	t.Helper()
	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	require.NoError(t, err)
	_, err = repo.CreateRemote(&gogitconfig.RemoteConfig{Name: "origin", URLs: []string{remoteURL}})
	require.NoError(t, err)
	return dir
}

// fakeSourceLister is a fake jules.sourceLister (unexported in package
// jules, satisfied structurally) so jules.NewJulesSourceRegistry can be
// exercised without a real HTTP round trip.
type FakeSourceLister struct {
	Sources []jules.JulesSource
	err     error
}

func (f *FakeSourceLister) ListSources(_ context.Context) ([]jules.JulesSource, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.Sources, nil
}

// newTestJulesSourceRegistry returns a *jules.JulesSourceRegistry that
// resolves "tstapler/stapler-squad" to "sources/github-tstapler-stapler-squad" —
// matching newTestJulesRepoWithRemote's default remote below.
func NewTestJulesSourceRegistry() *jules.JulesSourceRegistry {
	return jules.NewJulesSourceRegistry(&FakeSourceLister{
		Sources: []jules.JulesSource{{Name: "sources/github-tstapler-stapler-squad", ID: "src-1"}},
	})
}

// fakeJulesSessionCreator is a fake julesSessionCreator with a call counter
// and injectable error/delay/result, plus an optional beforeCreate hook so a
// test can inspect storage state mid-call (e.g. the reservation row's
// pending SessionUUID prefix, before CreateSession returns and
// DispatchToJules swaps it for the real name).
type FakeJulesSessionCreator struct {
	mu           sync.Mutex
	calls        int
	Delay        time.Duration
	Err          error
	ResultName   jules.JulesSessionName
	BeforeCreate func()
}

func (f *FakeJulesSessionCreator) CreateSession(_ context.Context, _ jules.CreateSessionRequest) (*jules.JulesSession, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.BeforeCreate != nil {
		f.BeforeCreate()
	}
	if f.Delay > 0 {
		time.Sleep(f.Delay) //nolint:notimesleeptest simulates a slow Jules API call whose wall-clock duration is under test
	}
	if f.Err != nil {
		return nil, f.Err
	}
	name := f.ResultName
	if name == "" {
		name = "sessions/xyz"
	}
	return &jules.JulesSession{Name: name, State: jules.JulesStateQueued}, nil
}

func (f *FakeJulesSessionCreator) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// newTestJulesConfig returns a *config.Config with Jules enabled,
// repoPath pre-acknowledged, and generous (but non-default) spend caps —
// the "everything else is satisfied" baseline most guard tests start from.
func NewTestJulesConfig(repoPath string) *config.Config {
	return &config.Config{
		Jules: config.JulesConfig{
			Enabled:                    true,
			EgressAcknowledgedRepos:    []string{repoPath},
			MaxConcurrentJulesSessions: 2,
			MaxJulesSessionsPerDay:     15,
		},
	}
}

// createTestJulesReadyItem creates a ready backlog item backed by a real git
// repo (remote "https://github.com/tstapler/stapler-squad.git") so
// resolveJulesOwnerRepo resolves it to tstapler/stapler-squad, matching
// newTestJulesSourceRegistry's registered source.
func CreateTestJulesReadyItem(t *testing.T, ctx context.Context, storage *session.Storage, title string) *backlogdto.BacklogItemData {
	t.Helper()
	repoPath := NewTestJulesRepoWithRemote(t, "https://github.com/tstapler/stapler-squad.git")
	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title:    title,
		Status:   string(domain.BacklogStatusReady),
		RepoPath: repoPath,
	})
	require.NoError(t, err)
	return item
}
