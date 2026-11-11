package prpoller

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
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
	session.SetPRUnverified(countUnverified(views))
	p.pruneStateBackoffs(views)
	due := p.selectDueStates(views, p.nowTime())
	if len(due) == 0 {
		return nil
	}
	var byRef map[session.PRRef][]*session.Instance
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
			ids := p.pollState(fctx, v, session.PRSourceTick, byRef)
			mu.Lock()
			terminal = append(terminal, ids...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return terminal
}

func countUnverified(views []session.PRView) int {
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
func (p *PRStatusPoller) selectDueStates(views []session.PRView, now time.Time) []session.PRView {
	limit := p.maxColdPolls()
	cold := 0
	var out []session.PRView
	for _, v := range views {
		if !p.stateDue(v, now) {
			continue
		}
		if v.State == session.PRStateUnknown || v.LastPolledAt.IsZero() {
			if cold >= limit {
				continue
			}
			cold++
		}
		out = append(out, v)
	}
	return out
}

func (p *PRStatusPoller) stateDue(v session.PRView, now time.Time) bool {
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

// pruneStateBackoffs drops retry gates of state rows no longer pollable (GC'd or terminal).
func (p *PRStatusPoller) pruneStateBackoffs(views []session.PRView) {
	live := make(map[int]struct{}, len(views))
	for _, v := range views {
		live[v.StateID] = struct{}{}
	}
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	for id := range p.stateBackoffs {
		if _, ok := live[id]; !ok {
			delete(p.stateBackoffs, id)
		}
	}
}

func (p *PRStatusPoller) clearStateBackoff(stateID int) {
	p.stateMu.Lock()
	delete(p.stateBackoffs, stateID)
	p.stateMu.Unlock()
}

// instancesByRef indexes the live sessions by each PR they link. It copies the
// instance list under p.mu and reads snapshots after releasing it.
func (p *PRStatusPoller) instancesByRef() map[session.PRRef][]*session.Instance {
	out := map[session.PRRef][]*session.Instance{}
	for _, inst := range p.GetInstances() {
		for _, pv := range inst.Snapshot().GitHub.PRs {
			out[pv.Ref] = append(out[pv.Ref], inst)
		}
	}
	return out
}

// pollState fetches one PR and records the outcome. No lock is held across the request.
func (p *PRStatusPoller) pollState(ctx context.Context, v session.PRView, src session.PRObservationSource, byRef map[session.PRRef][]*session.Instance) []int {
	observedAt := p.nowTime()
	ref, err := v.Ref.RepoRef()
	if err != nil {
		return nil
	}
	session.RecordPRPoll(v.State)
	info, changed, err := p.ghClient.GetPRInfoConditional(ctx, ref, v.Ref.Number(), p.etagCache)
	if err != nil {
		p.recordStateFailure(ctx, v, ref, observedAt, err, byRef)
		return nil
	}
	p.hostHealth.markSuccess(v.Ref.Host(), observedAt)
	p.clearStateBackoff(v.StateID)
	if !changed || info == nil {
		p.recordUnchanged(ctx, v, src, observedAt)
		if v.PollFailCount > 0 || v.PollError != "" {
			p.mirrorPollFacts(ctx, v.Ref, byRef)
		}
		return nil
	}
	return p.applyPRStateUpdate(ctx, v, info, src, observedAt, byRef)
}

// recordUnchanged clears stale failure bookkeeping after a 304; with nothing to
// clear it writes nothing.
func (p *PRStatusPoller) recordUnchanged(ctx context.Context, v session.PRView, src session.PRObservationSource, at time.Time) {
	if v.PollFailCount == 0 && v.PollError == "" || v.State == session.PRStateUnknown {
		return
	}
	obs := session.PRObservation{
		Source: src, Kind: session.PRObservationFetch, ObservedAt: at, State: v.State, IsDraft: v.IsDraft,
		CheckConclusion: v.CheckConclusion, ApprovedCount: v.ApprovedCount, ChangesRequestedCount: v.ChangesRequestedCount,
		URL: v.URL, Title: v.Title, HeadBranch: v.HeadBranch, BaseBranch: v.BaseBranch,
	}
	if _, err := p.storage.UpdatePullRequestState(ctx, v.Ref, obs); err != nil {
		log.Warn("PR status poller: clearing poll error failed", "pr", v.Ref, "err", err)
	}
}

func observationFromInfo(info *github.PRInfo, src session.PRObservationSource, at time.Time) (session.PRObservation, bool) {
	st, err := session.ParsePRState(strings.ToLower(info.State))
	if err != nil || st == session.PRStateUnknown {
		return session.PRObservation{}, false
	}
	obs := session.PRObservation{
		Source: src, Kind: session.PRObservationFetch, ObservedAt: at, State: st, IsDraft: info.IsDraft,
		CheckConclusion: info.CheckConclusion, ApprovedCount: info.ApprovedCount,
		ChangesRequestedCount: info.ChangesRequestedCount, URL: info.HTMLURL, Title: info.Title,
		HeadBranch: info.HeadRef, BaseBranch: info.BaseRef, OpenedAt: info.CreatedAt,
	}
	// PRInfo carries no merged_at/closed_at; updatedAt is the terminal moment.
	switch st {
	case session.PRStateMerged:
		obs.MergedAt = info.UpdatedAt
	case session.PRStateClosed:
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
func (p *PRStatusPoller) applyPRStateUpdate(ctx context.Context, v session.PRView, info *github.PRInfo, src session.PRObservationSource, at time.Time, byRef map[session.PRRef][]*session.Instance) []int {
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
		p.notifyTerminal([]int{v.StateID})
		return []int{v.StateID}
	}
	return nil
}

// mirrorToSession copies an applied observation into one session's PR set and
// persists its derived terminal flag when that changed.
func (p *PRStatusPoller) mirrorToSession(inst *session.Instance, ref session.PRRef, obs session.PRObservation, info *github.PRInfo, changed bool) {
	priority := string(github.DerivePRPriority(info))
	before := inst.Snapshot().GitHub.GitHubPRStatusTerminal
	inst.ApplyPRUpdate(ref, session.PRStatusUpdate{
		State: string(obs.State), Priority: priority, CheckConclusion: obs.CheckConclusion, Mergeable: info.Mergeable,
		ApprovedCount: obs.ApprovedCount, ChangesReqCount: obs.ChangesRequestedCount, IsDraft: obs.IsDraft,
		Terminal: obs.State.Terminal(), Checks: info.Checks, Reviews: info.Reviews,
	})
	inst.MirrorPRPollFacts(ref, session.PRView{Title: obs.Title, LastPolledAt: obs.ObservedAt})
	if obs.State.Terminal() {
		inst.MarkPRTerminal(ref, obs.State)
	}
	if after := inst.Snapshot().GitHub.GitHubPRStatusTerminal; after != before && p.storage != nil {
		if err := p.storage.UpdateGitHubPRStatusTerminal(inst.Snapshot().Title, after); err != nil {
			log.Warn("PR status poller: persisting session terminal flag failed", "session", inst.Snapshot().Title, "err", err)
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
func (p *PRStatusPoller) recordStateFailure(ctx context.Context, v session.PRView, ref github.RepoRef, at time.Time, err error, byRef map[session.PRRef][]*session.Instance) {
	probe := func() (github.RepoProbe, error) {
		if pr, ok := p.ghClient.(repoProber); ok {
			return pr.ProbeRepo(ctx, ref)
		}
		return github.RepoProbe{Status: 200}, nil
	}
	f := classifyPollError(err, v.Ref.Host(), at, p.hostHealth, probe)
	session.RecordPRPollFailure(f.Metric)
	obs := session.PRObservation{Source: session.PRSourceTick, Kind: session.PRObservationErrorNote, ObservedAt: at, PollError: f.PollError}
	if f.Counted() {
		obs.Kind = session.PRObservationCountedFailure
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
	if res.Applied {
		p.mirrorPollFacts(ctx, v.Ref, byRef)
	}
	if f.Counted() && res.Applied {
		p.notifyIfNowUnknown(ctx, v)
	}
}

// mirrorPollFacts pushes the row's fresh failure bookkeeping onto the linked live
// sessions so the verify-attempt counter the UI shows moves with each poll.
func (p *PRStatusPoller) mirrorPollFacts(ctx context.Context, ref session.PRRef, byRef map[session.PRRef][]*session.Instance) {
	if len(byRef[ref]) == 0 {
		return
	}
	fresh, err := p.storage.GetPullRequestState(ctx, ref)
	if err != nil {
		return
	}
	for _, inst := range byRef[ref] {
		inst.MirrorPRPollFacts(ref, fresh)
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
func (p *PRStatusPoller) notifyIfNowUnknown(ctx context.Context, before session.PRView) {
	if before.State == session.PRStateUnknown {
		return
	}
	after, err := p.storage.GetPullRequestState(ctx, before.Ref)
	if err != nil || after.State != session.PRStateUnknown {
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
func (p *PRStatusPoller) SetOnPRUnknown(fn func(session.PRView)) {
	p.mu.Lock()
	p.onPRUnknown = fn
	p.mu.Unlock()
}

// terminalDiscoveryFactor slows branch discovery for a session whose PRs are all terminal.
const terminalDiscoveryFactor = 10

func (p *PRStatusPoller) terminalDiscoveryBackoff() time.Duration {
	return p.config.NoPRBackoff * terminalDiscoveryFactor
}

func (p *PRStatusPoller) terminalDiscoveryDue(key string, now time.Time) bool {
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	return !now.Before(p.terminalDiscoveryAfter[key])
}

func (p *PRStatusPoller) armTerminalDiscovery(key string, now time.Time) {
	if p.config.NoPRBackoff <= 0 {
		return
	}
	p.stateMu.Lock()
	p.terminalDiscoveryAfter[key] = now.Add(p.terminalDiscoveryBackoff())
	p.stateMu.Unlock()
}

// discoveryEligible: a live, unarchived session whose links are all terminal. It
// polls nothing else, so a new PR on its branch is the only thing that can resume it.
func discoveryEligible(snap *session.InstanceSnapshot) bool {
	gh := snap.GitHub
	if snap.Status.IsSuspended() || snap.ArchivedAt != nil || gh.GitHubIsFork ||
		gh.GitHubOwner == "" || gh.GitHubRepo == "" || len(gh.PRs) == 0 {
		return false
	}
	for _, v := range gh.PRs {
		if !v.Terminal() {
			return false
		}
	}
	return true
}

// discoverForResolvedSessions looks for a new PR on the branch of each session
// whose PRs are all terminal, at terminalDiscoveryBackoff cadence (T3.2.3).
func (p *PRStatusPoller) discoverForResolvedSessions(ctx context.Context) {
	now := p.nowTime()
	for _, inst := range p.GetInstances() {
		snap := inst.Snapshot()
		if !discoveryEligible(snap) {
			continue
		}
		key := pollKey(snap)
		if !p.terminalDiscoveryDue(key, now) {
			continue
		}
		p.armTerminalDiscovery(key, now)
		fctx, cancel := context.WithTimeout(ctx, p.config.CallTimeout)
		fctx = github.WithGitHubCallOrigin(fctx, github.OriginPRStatusPoller)
		p.discoverBranchPR(fctx, inst, snap)
		cancel()
	}
}

// discoverBranchPR attaches the open PR on the session's current branch when it
// is not linked yet. AttachPR honors tombstones and the 50-link cap, and a new
// open PR makes the mirror's terminal flag false again, resuming the session.
func (p *PRStatusPoller) discoverBranchPR(ctx context.Context, inst *session.Instance, snap *session.InstanceSnapshot) {
	branch := inst.CurrentBranch()
	if branch == "" {
		return
	}
	gh := snap.GitHub
	ref, err := github.NewRepoRefWithHost(gh.GitHubOwner, gh.GitHubRepo, gh.GitHubHost)
	if err != nil {
		return
	}
	listKey := gh.GitHubOwner + "/" + gh.GitHubRepo + "/" + branch
	var etag string
	if v, ok := p.listEtags.Load(listKey); ok {
		etag = v.(string)
	}
	session.RecordPRDiscoveryListCall()
	info, newEtag, changed, err := p.ghClient.GetPRForBranchConditional(ctx, ref, branch, etag)
	// The ETag is stored only once the answer was handled: storing it before a
	// failed attach would make the next poll a 304 and the PR never discovered.
	storeEtag := func() {
		if newEtag != "" {
			p.listEtags.Store(listKey, newEtag)
		}
	}
	if err != nil || !changed || info == nil {
		if err == nil || errors.Is(err, github.ErrNoPR) {
			storeEtag()
		} else {
			log.Warn("PR status poller: discovery for a resolved session failed", "session", snap.Title, "err", err)
		}
		return
	}
	prRef, err := session.NewPRRef(gh.GitHubHost, gh.GitHubOwner, gh.GitHubRepo, info.Number)
	if err != nil {
		storeEtag()
		return
	}
	if _, linked := session.FindPR(gh.PRs, prRef); linked {
		storeEtag()
		return
	}
	initial := session.PRInitial{State: session.PRStateOpen, URL: info.HTMLURL, Title: info.Title, HeadBranch: info.HeadRef, BaseBranch: info.BaseRef, IsDraft: info.IsDraft}
	if err := inst.AttachPR(prRef, session.AttachSourceBranchDiscovery, initial); err != nil {
		log.Warn("PR status poller: attaching the discovered PR failed", "session", snap.Title, "pr", prRef, "err", err)
		return
	}
	storeEtag()
	log.Info("PR status poller: resolved session resumed, new PR on its branch", "session", snap.Title, "pr", prRef)
}

// Refresh fetches one PR through the poll core and returns once the observation
// was applied, dropped or failed, bounded by refreshTimeout. It never writes a
// row itself. A PR with no state row is an error.
func (p *PRStatusPoller) Refresh(ctx context.Context, ref session.PRRef, src session.PRObservationSource) error {
	if p.storage == nil {
		return domain.ErrNotFound
	}
	v, err := p.storage.GetPullRequestState(ctx, ref)
	if err != nil {
		return err
	}
	fctx, cancel := context.WithTimeout(ctx, refreshTimeout)
	defer cancel()
	fctx = github.WithGitHubCallOrigin(fctx, github.OriginWebhookReconcile)
	var byRef map[session.PRRef][]*session.Instance
	if p.multiPRScope(config.SessionMultiPRScopePoller) {
		byRef = p.instancesByRef()
	}
	p.pollState(fctx, v, src, byRef)
	return fctx.Err()
}

// refreshTimeout bounds one webhook or reconciler refresh.
const refreshTimeout = 10 * time.Second
