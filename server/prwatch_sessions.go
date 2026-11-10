package server

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/config"
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

// prwatchRemoteResolver resolves owner/repo/host from a session's git remote
// without blocking. resolved=false means "not known yet": the caller must not
// cache it and asks again on a later poll.
type prwatchRemoteResolver func(path string) (owner, repo, host string, resolved bool)

// detectRemote reads the worktree enricher's cache and queues a probe on a miss.
// DetectWorktree's git-ref open() can hang on a TCC/iCloud path (ADR-038), so the
// poller never calls it directly. Without a registered enricher nothing can bound
// the probe, so the path stays unresolved.
func detectRemote(path string) (owner, repo, host string, resolved bool) {
	if path == "" {
		return "", "", "", true
	}
	e := session.ActiveWorktreeEnricher()
	if e == nil {
		return "", "", "", false
	}
	res, ok := e.Result(path)
	if !ok {
		e.Request(path)
		return "", "", "", false
	}
	if res.Err != nil {
		return "", "", "", false // retried once the cached error expires
	}
	if res.Info == nil {
		return "", "", "", true
	}
	return res.Info.GitHubOwner, res.Info.GitHubRepo, res.Info.GitHubHost, true
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
	// perPR reports whether reader:poller is effective: one entry per open PR
	// instead of one per session, so a merged PR cannot hide an open one.
	perPR func() bool

	mu       sync.Mutex
	resolved map[string][3]string // uuid -> owner, repo, host; resolved once
}

var _ prwatch.Sessions = (*prwatchSessions)(nil)

func newPRWatchSessions(snapshots func() []*session.InstanceSnapshot, view prwatchSessionView, store prwatchBacklogStore, ignoreRepos []string) *prwatchSessions {
	return &prwatchSessions{
		snapshots: snapshots, view: view, store: store, ignoreRepos: ignoreRepos,
		resolve: detectRemote, clock: time.Now,
		tracker: prwatch.NewResumeTracker(), resolved: map[string][3]string{},
		perPR: func() bool { return config.SessionMultiPR(config.SessionMultiPRScopePoller) },
	}
}

func (s *prwatchSessions) List(ctx context.Context) ([]prwatch.SessionRef, error) {
	snaps := s.snapshots()
	facts := make([]prwatch.SessionFacts, 0, len(snaps))
	perPR := s.perPR()
	for _, sn := range snaps {
		facts = append(facts, s.factsFor(sn, perPR)...)
	}
	return prwatch.ComputeWatchSet(ctx, prwatch.WatchSetInput{
		Sessions: facts, IgnoreRepos: s.ignoreRepos,
		Links: &prwatchBacklogLinks{store: s.store}, Tracker: s.tracker, Now: s.clock(),
	}), nil
}

// factsFor returns the session's watch facts: one per open PR when perPR is on
// and the session has any, else the single legacy-scalar fact (an all-terminal
// session stays one entry so its skip reason still reads PR-terminal).
func (s *prwatchSessions) factsFor(sn *session.InstanceSnapshot, perPR bool) []prwatch.SessionFacts {
	base := s.factsOf(sn)
	if !perPR {
		return []prwatch.SessionFacts{base}
	}
	var out []prwatch.SessionFacts
	for _, v := range sn.GitHub.PRs {
		if v.Terminal() {
			continue
		}
		f := base
		f.Owner, f.Repo, f.Host = v.Ref.Owner(), v.Ref.Repo(), v.Ref.Host()
		f.PR, f.Terminal = v.Ref.Number(), false
		out = append(out, f)
	}
	if len(out) == 0 {
		return []prwatch.SessionFacts{base}
	}
	return out
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

// remoteOf holds s.mu only for map access, never across the resolver, so one
// slow path cannot stall List or any other session's resolution.
func (s *prwatchSessions) remoteOf(sn *session.InstanceSnapshot) (owner, repo, host string) {
	s.mu.Lock()
	r, ok := s.resolved[sn.UUID]
	s.mu.Unlock()
	if ok {
		return r[0], r[1], r[2]
	}
	path := sn.WorkingDir
	if path == "" {
		path = sn.Path
	}
	o, rp, h, resolved := s.resolve(path)
	if resolved {
		s.mu.Lock()
		s.resolved[sn.UUID] = [3]string{o, rp, h}
		s.mu.Unlock()
	}
	return o, rp, h
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
