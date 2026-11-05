package session

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
)

// pr_state_poller.go is the PR-keyed half of PRStatusPoller: it polls
// PullRequestState rows (one fetch per PR, whoever links it) instead of sessions.
// It always refreshes rows (the flag-independent refresher); under reader:poller it
// also fans each result out to the linked live sessions.

const (
	defaultMaxColdPollsPerTick = 20
	maxPollBackoff             = 6 * time.Hour
)

// repoProber is implemented by clients that can answer the repository metadata GET.
type repoProber interface {
	ProbeRepo(ctx context.Context, ref github.RepoRef) (github.RepoProbe, error)
}

func (realGHClient) ProbeRepo(ctx context.Context, ref github.RepoRef) (github.RepoProbe, error) {
	return github.ProbeRepo(ctx, ref)
}

// stateBackoff is the in-memory retry gate of a state that failed without counting.
type stateBackoff struct {
	failures int
	nextAt   time.Time
}

// pollBackoffDelay doubles with the failure count: one tick, two, four, ... capped.
func pollBackoffDelay(tick time.Duration, failures int) time.Duration {
	if failures <= 0 {
		return 0
	}
	d := tick
	for i := 1; i < failures && d < maxPollBackoff; i++ {
		d *= 2
	}
	if d > maxPollBackoff {
		d = maxPollBackoff
	}
	return d
}

func (p *PRStatusPoller) nowTime() time.Time {
	if p.clock != nil {
		return p.clock()
	}
	return time.Now()
}

func (p *PRStatusPoller) maxColdPolls() int {
	if p.config.MaxColdPollsPerTick > 0 {
		return p.config.MaxColdPollsPerTick
	}
	return defaultMaxColdPollsPerTick
}

// refresherEnabled reads the live kill switch and logs once per change.
func (p *PRStatusPoller) refresherEnabled() bool {
	on := p.refresherOn == nil || p.refresherOn()
	last := int32(0)
	if on {
		last = 1
	}
	if p.refresherState.Swap(last+1) != last+1 {
		log.Info("PR state refresher switched", "enabled", on)
	}
	return on
}

// pollPullRequestStates fetches every due PullRequestState once. fanOut mirrors
// each applied result into the linked live sessions. It returns the ids of the
// states that just became terminal (the seam for the cleanup listener, T5.4.3).
func (p *PRStatusPoller) pollPullRequestStates(ctx context.Context, fanOut bool) []int {
	if p.storage == nil {
		return nil
	}
	views, err := p.storage.ListPollableStates(ctx)
	if err != nil {
		log.Warn("PR status poller: list pollable states failed", "err", err)
		return nil
	}
	prMetrics.setUnverified(countUnverified(views))
	due := p.selectDueStates(views, p.nowTime())
	if len(due) == 0 {
		return nil
	}
	var byRef map[PRRef][]*Instance
	if fanOut {
		byRef = p.instancesByRef()
	}
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		terminal []int
		sem      = make(chan struct{}, max(p.config.ConcurrentFetches, 1))
	)
	for _, v := range due {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			fctx, cancel := context.WithTimeout(ctx, p.config.CallTimeout)
			defer cancel()
			fctx = github.WithGitHubCallOrigin(fctx, github.OriginPRStatusPoller)
			ids := p.pollState(fctx, v, PRSourceTick, byRef)
			mu.Lock()
			terminal = append(terminal, ids...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return terminal
}

func countUnverified(views []PRView) int {
	n := 0
	for _, v := range views {
		if v.UnverifiedTerminal() {
			n++
		}
	}
	return n
}

// selectDueStates applies the backoff gates and the cold-start cap. views arrive
// non-terminal first, then unverified terminal, each newest attach first; the cap
// keeps that order, so blocking rows resolve before seeded ones.
func (p *PRStatusPoller) selectDueStates(views []PRView, now time.Time) []PRView {
	limit := p.maxColdPolls()
	cold := 0
	var out []PRView
	for _, v := range views {
		if !p.stateDue(v, now) {
			continue
		}
		if v.State == PRStateUnknown || v.LastPolledAt.IsZero() {
			if cold >= limit {
				continue
			}
			cold++
		}
		out = append(out, v)
	}
	return out
}

func (p *PRStatusPoller) stateDue(v PRView, now time.Time) bool {
	if now.Before(v.RateLimitResetAt) {
		return false
	}
	if v.PollFailCount > 0 && !v.LastPolledAt.IsZero() &&
		now.Before(v.LastPolledAt.Add(pollBackoffDelay(p.config.PollInterval, v.PollFailCount))) {
		return false
	}
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	b, ok := p.stateBackoffs[v.StateID]
	return !ok || !now.Before(b.nextAt)
}

func (p *PRStatusPoller) noteNonCountedFailure(stateID int, now time.Time) {
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	b := p.stateBackoffs[stateID]
	b.failures++
	b.nextAt = now.Add(pollBackoffDelay(p.config.PollInterval, b.failures))
	p.stateBackoffs[stateID] = b
}

func (p *PRStatusPoller) clearStateBackoff(stateID int) {
	p.stateMu.Lock()
	delete(p.stateBackoffs, stateID)
	p.stateMu.Unlock()
}

// instancesByRef indexes the live sessions by each PR they link. It copies the
// instance list under p.mu and reads snapshots after releasing it.
func (p *PRStatusPoller) instancesByRef() map[PRRef][]*Instance {
	out := map[PRRef][]*Instance{}
	for _, inst := range p.GetInstances() {
		for _, pv := range inst.Snapshot().GitHub.PRs {
			out[pv.Ref] = append(out[pv.Ref], inst)
		}
	}
	return out
}

// pollState fetches one PR and records the outcome. No lock is held across the request.
func (p *PRStatusPoller) pollState(ctx context.Context, v PRView, src PRObservationSource, byRef map[PRRef][]*Instance) []int {
	observedAt := p.nowTime()
	ref, err := v.Ref.repoRef()
	if err != nil {
		return nil
	}
	prMetrics.poll(v.State)
	info, changed, err := p.ghClient.GetPRInfoConditional(ctx, ref, v.Ref.Number(), p.etagCache)
	if err != nil {
		p.recordStateFailure(ctx, v, ref, observedAt, err)
		return nil
	}
	p.hostHealth.markSuccess(v.Ref.Host(), observedAt)
	p.clearStateBackoff(v.StateID)
	if !changed || info == nil {
		p.recordUnchanged(ctx, v, src, observedAt)
		return nil
	}
	return p.applyPRStateUpdate(ctx, v, info, src, observedAt, byRef)
}

// recordUnchanged clears stale failure bookkeeping after a 304; with nothing to
// clear it writes nothing.
func (p *PRStatusPoller) recordUnchanged(ctx context.Context, v PRView, src PRObservationSource, at time.Time) {
	if v.PollFailCount == 0 && v.PollError == "" || v.State == PRStateUnknown {
		return
	}
	obs := PRObservation{
		Source: src, Kind: PRObservationFetch, ObservedAt: at, State: v.State, IsDraft: v.IsDraft,
		CheckConclusion: v.CheckConclusion, ApprovedCount: v.ApprovedCount, ChangesRequestedCount: v.ChangesRequestedCount,
		URL: v.URL, HeadBranch: v.HeadBranch, BaseBranch: v.BaseBranch,
	}
	if _, err := p.storage.UpdatePullRequestState(ctx, v.Ref, obs); err != nil {
		log.Warn("PR status poller: clearing poll error failed", "pr", v.Ref, "err", err)
	}
}

func observationFromInfo(info *github.PRInfo, src PRObservationSource, at time.Time) (PRObservation, bool) {
	st, err := ParsePRState(strings.ToLower(info.State))
	if err != nil || st == PRStateUnknown {
		return PRObservation{}, false
	}
	obs := PRObservation{
		Source: src, Kind: PRObservationFetch, ObservedAt: at, State: st, IsDraft: info.IsDraft,
		CheckConclusion: info.CheckConclusion, ApprovedCount: info.ApprovedCount,
		ChangesRequestedCount: info.ChangesRequestedCount, URL: info.HTMLURL,
		HeadBranch: info.HeadRef, BaseBranch: info.BaseRef, OpenedAt: info.CreatedAt,
	}
	// PRInfo carries no merged_at/closed_at; updatedAt is the terminal moment.
	switch st {
	case PRStateMerged:
		obs.MergedAt = info.UpdatedAt
	case PRStateClosed:
		obs.ClosedAt = info.UpdatedAt
	}
	return obs, true
}

// applyPRStateUpdate writes the state row once, then mirrors the result into each
// linked live session. There is NO transaction around the two steps: a crash
// between them leaves the row ahead of some session mirrors, which the always-on
// mirror reconciler (T1.5.2) repairs; each session persist is serialized per
// session so an older snapshot cannot land after a newer one. It returns the ids
// of states that just went terminal and calls nothing else (T5.4.3 adds the listener).
func (p *PRStatusPoller) applyPRStateUpdate(ctx context.Context, v PRView, info *github.PRInfo, src PRObservationSource, at time.Time, byRef map[PRRef][]*Instance) []int {
	obs, ok := observationFromInfo(info, src, at)
	if !ok {
		log.Warn("PR status poller: unusable PR state", "pr", v.Ref, "state", info.State)
		return nil
	}
	res, err := p.storage.UpdatePullRequestState(ctx, v.Ref, obs)
	if err != nil {
		log.Warn("PR status poller: updating PR state failed", "pr", v.Ref, "err", err)
		return nil
	}
	if !res.Applied {
		return nil
	}
	changed := v.State != obs.State || v.CheckConclusion != obs.CheckConclusion ||
		v.ApprovedCount != obs.ApprovedCount || v.ChangesRequestedCount != obs.ChangesRequestedCount || v.IsDraft != obs.IsDraft
	for _, inst := range byRef[v.Ref] {
		p.mirrorToSession(inst, v.Ref, obs, info, changed)
	}
	if obs.State.Terminal() && !v.Terminal() {
		return []int{v.StateID}
	}
	return nil
}

// mirrorToSession copies an applied observation into one session's PR set and
// persists its derived terminal flag when that changed.
func (p *PRStatusPoller) mirrorToSession(inst *Instance, ref PRRef, obs PRObservation, info *github.PRInfo, changed bool) {
	priority := string(github.DerivePRPriority(info))
	before := inst.Snapshot().GitHub.GitHubPRStatusTerminal
	inst.ApplyPRUpdate(ref, PRStatusUpdate{
		State: string(obs.State), Priority: priority, CheckConclusion: obs.CheckConclusion, Mergeable: info.Mergeable,
		ApprovedCount: obs.ApprovedCount, ChangesReqCount: obs.ChangesRequestedCount, IsDraft: obs.IsDraft,
		Terminal: obs.State.Terminal(), Checks: info.Checks, Reviews: info.Reviews,
	})
	if obs.State.Terminal() {
		inst.MarkPRTerminal(ref, obs.State)
	}
	if after := inst.Snapshot().GitHub.GitHubPRStatusTerminal; after != before && p.storage != nil {
		if err := p.storage.UpdateGitHubPRStatusTerminal(inst.Snapshot().Title, after); err != nil {
			log.Warn("PR status poller: persisting session terminal flag failed", "session", inst.Title, "err", err)
		}
	}
	if changed {
		p.mu.RLock()
		onUpdated := p.onUpdated
		p.mu.RUnlock()
		if onUpdated != nil {
			onUpdated(inst)
		}
	}
}

// recordStateFailure classifies a failed fetch and records it on the row.
func (p *PRStatusPoller) recordStateFailure(ctx context.Context, v PRView, ref github.RepoRef, at time.Time, err error) {
	probe := func() (github.RepoProbe, error) {
		if pr, ok := p.ghClient.(repoProber); ok {
			return pr.ProbeRepo(ctx, ref)
		}
		return github.RepoProbe{Status: 200}, nil
	}
	f := classifyPollError(err, v.Ref.Host(), at, p.hostHealth, probe)
	prMetrics.failure(f.Metric)
	obs := PRObservation{Source: PRSourceTick, Kind: PRObservationErrorNote, ObservedAt: at, PollError: f.PollError}
	if f.Counted() {
		obs.Kind = PRObservationCountedFailure
	} else {
		p.noteNonCountedFailure(v.StateID, at)
	}
	if f.Class == pollFailRateLimit {
		obs.RateLimitResetAt = rateLimitResetAfter(at, p.config.PollInterval)
	}
	log.Warn("PR status poller: fetch failed", "pr", v.Ref, "class", f.PollError, "counted", f.Counted(), "err", err)
	res, uerr := p.storage.UpdatePullRequestState(ctx, v.Ref, obs)
	if uerr != nil {
		log.Warn("PR status poller: recording poll failure failed", "pr", v.Ref, "err", uerr)
		return
	}
	if f.Counted() && res.Applied {
		p.notifyIfNowUnknown(ctx, v)
	}
}

func rateLimitResetAfter(at time.Time, tick time.Duration) time.Time {
	if limited, until := github.DefaultRateLimiter.IsLimited(); limited && until.After(at) {
		return until
	}
	return at.Add(tick)
}

// notifyIfNowUnknown raises the one-shot unknown notification when a live
// (non-terminal) row has just exhausted its counted failures. Unverified terminal
// rows never notify.
func (p *PRStatusPoller) notifyIfNowUnknown(ctx context.Context, before PRView) {
	if before.State == PRStateUnknown {
		return
	}
	after, err := p.storage.GetPullRequestState(ctx, before.Ref)
	if err != nil || after.State != PRStateUnknown {
		return
	}
	log.Warn("PR status poller: PR state is unknown after repeated failures", "pr", before.Ref, "poll_error", after.PollError)
	p.mu.RLock()
	fn := p.onPRUnknown
	p.mu.RUnlock()
	if fn != nil {
		fn(after)
	}
}

// SetOnPRUnknown registers the callback raised once when a PR becomes unknown.
func (p *PRStatusPoller) SetOnPRUnknown(fn func(PRView)) {
	p.mu.Lock()
	p.onPRUnknown = fn
	p.mu.Unlock()
}
