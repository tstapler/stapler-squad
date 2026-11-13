package session

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/tstapler/stapler-squad/session/internal/sessiontest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// Tests of the terminal-scan PR writer (session-multi-pr-tracking T2.2.1).

const pushLine12 = "remote:      https://github.com/acme/app/pull/12\n"

func TestScanAndLinkPRURL_ShouldAttachAndClearTerminal_WhenNumberChanges(t *testing.T) {
	t.Parallel()
	t.Run("session with a uuid attaches #12 as push_scan beside #11 and mirrors terminal=false", func(t *testing.T) {
		inst := newPRTestInstance(t)
		fake := &fakePRPersister{}
		inst.SetPRPersister(fake)
		require.NoError(t, inst.AttachPR(mustPRRef(t, "", "acme", "app", 11), domain.AttachSourceBranchDiscovery,
			domain.PRInitial{State: domain.PRStateMerged, Terminal: true}))
		require.True(t, inst.Snapshot().GitHub.GitHubPRStatusTerminal)

		scanAndLinkPRURL(inst, true, scannedPRURLs{}, nil, pushLine12)

		prs := inst.Snapshot().GitHub.PRs
		require.Len(t, prs, 2)
		assert.Equal(t, 12, prs[1].Ref.Number())
		assert.Equal(t, domain.AttachSourcePushScan, prs[1].AttachSource)
		g := inst.Snapshot().GitHub
		assert.Equal(t, 12, g.GitHubPRNumber)
		assert.False(t, g.GitHubPRStatusTerminal, "merged #11's true must not ride onto open #12")
		require.Equal(t, 2, fake.callCount())
		assert.False(t, fake.calls[1].mirror.Terminal, "the persisted mirror carries the cleared flag")
	})

	t.Run("session without a uuid clears the stored flag at once through the title-keyed write", func(t *testing.T) {
		inst := legacyOnlyScanInstance(t)
		fake := &fakePRPersister{}
		inst.SetPRPersister(fake)

		scanAndLinkPRURL(inst, true, scannedPRURLs{}, nil, pushLine12)

		g := inst.Snapshot().GitHub
		assert.Equal(t, 12, g.GitHubPRNumber)
		assert.False(t, g.GitHubPRStatusTerminal)
		assert.Empty(t, g.PRs, "no uuid: no link")
		assert.Equal(t, 0, fake.callCount())
		require.Len(t, fake.terminalWrites, 1)
		assert.Equal(t, terminalWrite{title: "legacy-scan", terminal: false}, fake.terminalWrites[0])
		assert.False(t, inst.ToInstanceData().TerminalClearPending)
	})

	t.Run("a failed immediate clear leaves terminalClearPending for the next full save", func(t *testing.T) {
		inst := legacyOnlyScanInstance(t)
		inst.SetPRPersister(&fakePRPersister{terminalErr: errors.New("database is locked")})

		scanAndLinkPRURL(inst, true, scannedPRURLs{}, nil, pushLine12)

		assert.True(t, inst.ToInstanceData().TerminalClearPending)
	})
}

func legacyOnlyScanInstance(t *testing.T) *Instance {
	t.Helper()
	inst := &Instance{
		Title: "legacy-scan", Path: t.TempDir(), Program: "echo", Branch: "feat/x",
		GitHubOwner: "acme", GitHubRepo: "app",
		GitHubPRURL: "https://github.com/acme/app/pull/11", GitHubPRNumber: 11, GitHubPRStatusTerminal: true,
	}
	FinishInstanceConstruction(inst)
	return inst
}

func TestScanAndLinkPRURL_ShouldNotAttach_WhenRepoMismatchOrBareMention(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		configure func(*Instance)
		output    string
		wantDebug bool
	}{
		{name: "other repo", output: "remote: https://github.com/other/lib/pull/7\n", wantDebug: true},
		{name: "other host", configure: func(i *Instance) { i.GitHubHost = "ghe.example.com" }, output: pushLine12, wantDebug: true},
		{name: "bare mention in non-push output", output: "I opened https://github.com/acme/app/pull/12 earlier\n"},
		{name: "session repo unresolved", configure: func(i *Instance) { i.GitHubOwner, i.GitHubRepo = "", "" }, output: pushLine12, wantDebug: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := sessiontest.InstallRecordingHandler(t)
			inst := &Instance{Title: "scan-mismatch", UUID: "u-mismatch", Path: t.TempDir(), Program: "echo", GitHubOwner: "acme", GitHubRepo: "app"}
			if tt.configure != nil {
				tt.configure(inst)
			}
			FinishInstanceConstruction(inst)

			scanAndLinkPRURL(inst, true, scannedPRURLs{}, nil, tt.output)

			assert.Empty(t, inst.Snapshot().GitHub.PRs)
			assert.Zero(t, inst.Snapshot().GitHub.GitHubPRNumber)
			var debugNamesURL bool
			for _, r := range h.RecordsAtLevel(slog.LevelDebug) {
				if _, ok := r.Attrs["url"]; ok {
					debugNamesURL = true
				}
			}
			assert.Equal(t, tt.wantDebug, debugNamesURL, "a mismatch is logged at debug with the URL; a bare mention is silent")
		})
	}
}

func TestScanAndLinkPRURL_ShouldAttachEachURLOnce_WhenTicksRepeatAndSecondURLAppears(t *testing.T) {
	t.Parallel()
	inst := newPRTestInstance(t)
	fake := &fakePRPersister{}
	inst.SetPRPersister(fake)
	seen := scannedPRURLs{}

	scanAndLinkPRURL(inst, true, seen, nil, pushLine12)
	scanAndLinkPRURL(inst, true, seen, nil, pushLine12)
	assert.Equal(t, 1, fake.callCount(), "a URL still in the scrollback is not re-processed")

	scanAndLinkPRURL(inst, true, seen, nil, pushLine12+"remote:      https://github.com/acme/app/pull/13\n")
	assert.Equal(t, []int{12, 13}, attachNumbers(t, inst))

	scanAndLinkPRURL(inst, false, scannedPRURLs{}, nil, "remote: https://github.com/acme/app/pull/14\n")
	assert.Equal(t, []int{12, 13}, attachNumbers(t, inst), "nothing is scanned before the initial prompt is sent")
}

func TestScanAndLinkPRURL_ShouldUseCanonicalIdentity_WhenURLCasingDiffers(t *testing.T) {
	t.Parallel()
	storage, inst := persistedPRSession(t)
	inst.SetPRPersister(storage)

	scanAndLinkPRURL(inst, true, scannedPRURLs{}, nil, "remote: https://github.com/Acme/App/pull/12\n")
	assert.Equal(t, 1, countStates(t, storage.Repo))
	assert.Equal(t, 1, countLinks(t, storage.Repo))
	require.Equal(t, []int{12}, attachNumbers(t, inst))
	assert.Equal(t, "acme", inst.Snapshot().GitHub.PRs[0].Ref.Owner())
}

func TestScanAndLinkPRURL_ShouldSkipTombstonedRef_WhenOwnerDetachedItBefore(t *testing.T) {
	t.Parallel()
	storage, inst := persistedPRSession(t)
	inst.SetPRPersister(storage)
	ctx := context.Background()
	owner := SessionPROwner(inst.UUID)
	_, err := storage.Repo.UpsertSessionPR(ctx, owner, mustPRRef(t, "", "acme", "app", 12), domain.PRLink{Source: domain.AttachSourceManual})
	require.NoError(t, err)
	_, err = storage.Repo.DeleteSessionPR(ctx, owner, mustPRRef(t, "", "Acme", "App", 12))
	require.NoError(t, err)
	require.Equal(t, 0, countLinks(t, storage.Repo))

	scanAndLinkPRURL(inst, true, scannedPRURLs{}, nil, "remote: https://github.com/Acme/App/pull/12\n")

	assert.Equal(t, 0, countLinks(t, storage.Repo), "a tombstone for Acme/App#12 suppresses the scan")
	assert.Empty(t, inst.Snapshot().GitHub.PRs)
}

// failingTerminalPersister fails the title-keyed terminal write once, like a busy database.
type failingTerminalPersister struct {
	*Storage
	failed bool
}

func (f *failingTerminalPersister) UpdateGitHubPRStatusTerminal(title string, terminal bool) error {
	if !f.failed {
		f.failed = true
		return errors.New("database is locked")
	}
	return f.Storage.UpdateGitHubPRStatusTerminal(title, terminal)
}

func TestScanAndLinkPRURL_ShouldReloadTerminalFalse_WhenSessionHasNoLink(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		persist func(*Storage) prSetPersister
	}{
		{"immediate write", func(s *Storage) prSetPersister { return s }},
		{"retried by the next full save", func(s *Storage) prSetPersister { return &failingTerminalPersister{Storage: s} }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			storage, cleanup := createTestStorage(t)
			t.Cleanup(cleanup)
			prLoadFixture(t, storage, "legacy-scan", "", "acme", "app", 11)
			require.NoError(t, storage.Repo.UpdateGitHubPRStatusTerminal(context.Background(), "legacy-scan", true))
			loaded, err := storage.LoadInstances()
			require.NoError(t, err)
			require.Len(t, loaded, 1)
			inst := loaded[0]
			require.True(t, inst.Snapshot().GitHub.GitHubPRStatusTerminal)
			inst.SetPRPersister(tt.persist(storage))

			scanAndLinkPRURL(inst, true, scannedPRURLs{}, nil, pushLine12)
			require.NoError(t, storage.SaveInstances([]*Instance{inst}))

			again, err := storage.LoadInstances()
			require.NoError(t, err)
			require.Len(t, again, 1)
			g := again[0].Snapshot().GitHub
			assert.Equal(t, 12, g.GitHubPRNumber)
			assert.False(t, g.GitHubPRStatusTerminal, "the stored true must not survive the restart")
		})
	}
}
