package server

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/server/prwatch"
	"github.com/tstapler/stapler-squad/session"
)

// prwatchSessionView is the steerer subset the Sessions adapter needs;
// *services.SessionService satisfies it.
type prwatchSessionView interface {
	SessionProgram(sessionUUID string) (program string, ok bool)
	IsHiddenSession(sessionUUID string) bool
	IsReadyForSteer(sessionUUID string) bool
}

// prwatchBacklogStore is the Storage subset used for the D4 exclusion.
type prwatchBacklogStore interface {
	ListItemSessionsBySessionUUIDs(ctx context.Context, uuids []string) (map[string]session.ItemSessionSummary, error)
	GetBacklogItem(ctx context.Context, id string) (*session.BacklogItemData, error)
	GetItemSessionBySessionUUID(ctx context.Context, sessionUUID string) (session.ItemSessionSummary, error)
}

// prwatchRemoteResolver resolves owner/repo/host from a session's git remote.
type prwatchRemoteResolver func(path string) (owner, repo, host string, ok bool)

func detectRemote(path string) (string, string, string, bool) {
	if path == "" {
		return "", "", "", false
	}
	info, err := session.DetectWorktree(path)
	if err != nil || info == nil || info.GitHubOwner == "" || info.GitHubRepo == "" {
		return "", "", "", false
	}
	return info.GitHubOwner, info.GitHubRepo, info.GitHubHost, true
}

// prwatchSessions implements prwatch.Sessions over live session snapshots.
type prwatchSessions struct {
	snapshots   func() []*session.InstanceSnapshot
	view        prwatchSessionView
	store       prwatchBacklogStore
	ignoreRepos []string
	resolve     prwatchRemoteResolver
	clock       func() time.Time
	tracker     *prwatch.ResumeTracker

	mu       sync.Mutex
	resolved map[string][3]string // uuid -> owner, repo, host; resolved once
}

var _ prwatch.Sessions = (*prwatchSessions)(nil)

func newPRWatchSessions(snapshots func() []*session.InstanceSnapshot, view prwatchSessionView, store prwatchBacklogStore, ignoreRepos []string) *prwatchSessions {
	return &prwatchSessions{
		snapshots: snapshots, view: view, store: store, ignoreRepos: ignoreRepos,
		resolve: detectRemote, clock: time.Now,
		tracker: prwatch.NewResumeTracker(), resolved: map[string][3]string{},
	}
}

func (s *prwatchSessions) List(ctx context.Context) ([]prwatch.SessionRef, error) {
	snaps := s.snapshots()
	facts := make([]prwatch.SessionFacts, 0, len(snaps))
	for _, sn := range snaps {
		facts = append(facts, s.factsOf(sn))
	}
	return prwatch.ComputeWatchSet(ctx, prwatch.WatchSetInput{
		Sessions: facts, IgnoreRepos: s.ignoreRepos,
		Links: &prwatchBacklogLinks{store: s.store}, Tracker: s.tracker, Now: s.clock(),
	}), nil
}

func (s *prwatchSessions) factsOf(sn *session.InstanceSnapshot) prwatch.SessionFacts {
	gh := sn.GitHub
	f := prwatch.SessionFacts{
		UUID: sn.UUID, Title: sn.Title, Lifecycle: lifecycleOf(sn.Status), Hidden: sn.Hidden,
		Owner: gh.GitHubOwner, Repo: gh.GitHubRepo, Host: gh.GitHubHost,
		PR: gh.GitHubPRNumber, Fork: gh.GitHubIsFork, Terminal: gh.GitHubPRStatusTerminal,
	}
	if f.PR > 0 && (f.Owner == "" || f.Repo == "") && f.Lifecycle != prwatch.LifeGone {
		f.Owner, f.Repo, f.Host = s.remoteOf(sn)
	}
	return f
}

func (s *prwatchSessions) remoteOf(sn *session.InstanceSnapshot) (owner, repo, host string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.resolved[sn.UUID]; ok {
		return r[0], r[1], r[2]
	}
	path := sn.WorkingDir
	if path == "" {
		path = sn.Path
	}
	o, r, h, _ := s.resolve(path)
	s.resolved[sn.UUID] = [3]string{o, r, h}
	return o, r, h
}

func lifecycleOf(st session.Status) prwatch.Lifecycle {
	switch st {
	case session.Paused, session.Hibernated, session.Crashed:
		return prwatch.LifeSuspended
	case session.Stopped, session.PermanentlyFailed, session.Failed:
		return prwatch.LifeGone
	case session.Creating, session.Restoring:
		return prwatch.LifeTransitional
	}
	return prwatch.LifeLive
}

func (s *prwatchSessions) Program(sessionUUID string) string {
	p, _ := s.view.SessionProgram(sessionUUID)
	return p
}

func (s *prwatchSessions) IsHidden(sessionUUID string) bool {
	return s.view.IsHiddenSession(sessionUUID)
}

// IsBusy is the negation of IsReadyForSteer (D17), so a session with no live
// instance or status source also reads as busy.
func (s *prwatchSessions) IsBusy(sessionUUID string) bool {
	return !s.view.IsReadyForSteer(sessionUUID)
}

// prwatchBacklogLinks implements prwatch.BacklogLinks (D4): one batched link
// query, then at most one GetBacklogItem per distinct linked item.
type prwatchBacklogLinks struct{ store prwatchBacklogStore }

func (l *prwatchBacklogLinks) Pending(ctx context.Context, uuids []string) (pending, failed map[string]bool, err error) {
	links, err := l.store.ListItemSessionsBySessionUUIDs(ctx, uuids)
	if err != nil {
		return nil, nil, err
	}
	pending, failed = map[string]bool{}, map[string]bool{}
	type verdict struct {
		pending bool
		failed  bool
	}
	byItem := map[string]verdict{}
	for sessionUUID, link := range links {
		itemID := link.BacklogItemID
		if itemID == "" {
			// The batched query does not load the BacklogItem edge
			// (session/storage_backlog.go:408-423), so BacklogItemID is empty;
			// the single-session query does load it.
			one, oerr := l.store.GetItemSessionBySessionUUID(ctx, sessionUUID)
			if oerr != nil || one.BacklogItemID == "" {
				failed[sessionUUID] = true
				continue
			}
			itemID = one.BacklogItemID
		}
		v, seen := byItem[itemID]
		if !seen {
			item, gerr := l.store.GetBacklogItem(ctx, itemID)
			switch {
			case errors.Is(gerr, session.ErrNotFound):
				// Dangling link: nothing else steers it, keep watching.
			case gerr != nil || item == nil:
				v.failed = true
			default:
				v.pending = item.Status == string(session.BacklogStatusPRPending)
			}
			byItem[itemID] = v
		}
		if v.failed {
			failed[sessionUUID] = true
		} else if v.pending {
			pending[sessionUUID] = true
		}
	}
	return pending, failed, nil
}
