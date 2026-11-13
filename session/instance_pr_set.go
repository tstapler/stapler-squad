package session

import (
	"context"
	"errors"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
)

// instance_pr_set.go holds the actor setters for a session's linked pull
// requests. Each one follows the setGitHubPRURLLocked protocol (lock, mutate,
// buildSnapshot, unlock, Store) and changes the in-memory set only. Storage is
// a separate step: AttachPR persists afterwards through prSetPersister, reading
// the values from Snapshot() at persist time. State columns are never written
// from here (invariant I1); ApplyPRUpdate and MarkPRTerminal only mirror an
// observation the poller already wrote to the state row.

// PRMirror is the legacy scalar PR columns on the session row.
type PRMirror struct {
	URL      string
	Number   int
	Terminal bool
}

func prMirrorOf(g domain.GitHubIntegration) PRMirror {
	return PRMirror{URL: g.GitHubPRURL, Number: g.GitHubPRNumber, Terminal: g.GitHubPRStatusTerminal}
}

// prSetPersister is the storage seam AttachPR persists through; *Storage is the
// only implementation.
type prSetPersister interface {
	AttachSessionPR(ctx context.Context, sessionUUID string, ref domain.PRRef, link PRLink, mirror PRMirror) (PRLinkResult, error)
	UpdateGitHubPRStatusTerminal(title string, terminal bool) error
	PersistPRMirror(sessionUUID, url string, number int, terminal bool) error
}

// SetPRPersister injects the storage backend AttachPR persists through.
func (i *Instance) SetPRPersister(p prSetPersister) {
	i.prPersistMu.Lock()
	i.prPersister = p
	i.prPersistMu.Unlock()
}

// PRPersistPending reports that a PR set change has not reached the database; the
// next full save writes it.
func (i *Instance) PRPersistPending() bool { return i.prPersistPending.Load() }

func (i *Instance) clearPRPersistPending() { i.prPersistPending.Store(false) }

// AttachPR upserts ref into the session's PR set. It is idempotent per ref, takes
// the role from AssignRole, and returns ErrNoSessionUUID (changing nothing) for a
// session without a uuid so the caller can use the legacy scalar path. The
// database write follows the actor call; a failure or a not-yet-persisted session
// row only marks the set pending for the next full save.
func (i *Instance) AttachPR(ref domain.PRRef, source domain.AttachSource, initial PRInitial) error {
	if ref.IsZero() {
		return ErrZeroPRRef
	}
	out, err := sendSyncValue(i, func(s *instanceState) (attachOutcome, error) {
		view, added, err := attachPRLocked(s.inst, ref, source, initial)
		return attachOutcome{view: view, added: added}, err
	})
	if err != nil || !out.added {
		return err
	}
	i.persistPRAttach(out.view)
	log.Info("session pr attached", "session", i.Snapshot().Title, "pr", ref.String(), "role", out.view.Role, "source", source)
	return nil
}

type attachOutcome struct {
	view  domain.PRView
	added bool
}

func attachPRLocked(inst *Instance, ref domain.PRRef, source domain.AttachSource, initial PRInitial) (domain.PRView, bool, error) {
	inst.mu.Lock()
	now := time.Now() // taken under the lock so AttachedAt order matches slice order
	if inst.UUID == "" {
		inst.mu.Unlock()
		return domain.PRView{}, false, ErrNoSessionUUID
	}
	if existing, ok := FindPR(inst.PRs, ref); ok {
		inst.mu.Unlock()
		return existing, false, nil
	}
	if len(inst.PRs) >= maxSessionPRLinks {
		inst.mu.Unlock()
		return domain.PRView{}, false, ErrSessionPRLinkCap
	}
	before := activePRNumber(inst)
	view := domain.PRView{
		Ref: ref, State: initial.state(), StoredTerminal: initial.Terminal, URL: initial.URL, Title: initial.Title,
		HeadBranch: initial.HeadBranch, BaseBranch: initial.BaseBranch, IsDraft: initial.IsDraft,
		SessionUUID: inst.UUID, Role: domain.AssignRole(inst.PRs, initial.BaseBranch),
		AttachSource: source, AttachedAt: now,
	}
	inst.PRs = append(inst.PRs, view)
	inst.HasPRLinks = true
	prMetrics.observeSessionPRCount(len(inst.PRs))
	if inst.LegacyPRUnrepresented && inst.GitHubPRNumber == ref.Number() {
		inst.LegacyPRUnrepresented = false
	}
	publishPRSetLocked(inst, before)
	return view, true, nil
}

// ApplyPRUpdate overwrites ref's in-memory status with a poller observation (the
// existing PRStatusUpdate; an unparseable State leaves the state unchanged) and
// reports whether ref is in the set.
func (i *Instance) ApplyPRUpdate(ref domain.PRRef, u PRStatusUpdate) bool {
	return i.mutatePR(ref, func(v *domain.PRView) { applyUpdateToView(v, u) })
}

// MirrorPRPollFacts copies the poll bookkeeping and title of a fresh state row
// onto the live view, so the session's PR list reports verify attempts and the
// title without waiting for a reload. Zero fields in fresh clear the view's.
func (i *Instance) MirrorPRPollFacts(ref domain.PRRef, fresh domain.PRView) bool {
	return i.mutatePR(ref, func(v *domain.PRView) {
		if fresh.Title != "" {
			v.Title = fresh.Title
		}
		v.PollFailCount, v.FirstFailAt, v.PollError = fresh.PollFailCount, fresh.FirstFailAt, fresh.PollError
		v.LastPolledAt, v.RateLimitResetAt = fresh.LastPolledAt, fresh.RateLimitResetAt
	})
}

func applyUpdateToView(v *domain.PRView, u PRStatusUpdate) {
	if st, err := domain.ParsePRState(u.State); err == nil {
		v.State = st
	}
	v.StoredTerminal, v.IsDraft = u.Terminal, u.IsDraft
	v.CheckConclusion, v.ApprovedCount, v.ChangesRequestedCount = u.CheckConclusion, u.ApprovedCount, u.ChangesReqCount
	v.LastCheckedAt = time.Now()
}

// applyLegacyUpdateToActivePR carries a legacy per-session observation onto the
// active PR's view, so the snapshot's active-PR projection keeps it (with every
// scope off the legacy loop is the only thing that updates a session's PRs).
// The caller holds inst.mu.
func applyLegacyUpdateToActivePR(inst *Instance, u PRStatusUpdate) {
	if active := domain.ActivePR(inst.PRs, inst.Branch); active != nil {
		if idx := indexOfPR(inst.PRs, active.Ref); idx >= 0 {
			applyUpdateToView(&inst.PRs[idx], u)
		}
	}
}

// MarkPRTerminal records that ref reached a closed or merged state.
func (i *Instance) MarkPRTerminal(ref domain.PRRef, state domain.PRState) bool {
	if !state.Terminal() {
		return false
	}
	return i.mutatePR(ref, func(v *domain.PRView) { v.State, v.StoredTerminal = state, true })
}

// PinPR pins ref as the session's active PR (clearing every other pin) or unpins it.
func (i *Instance) PinPR(ref domain.PRRef, pinned bool) bool {
	found, _ := sendSyncValue(i, func(s *instanceState) (bool, error) {
		inst := s.inst
		inst.mu.Lock()
		if indexOfPR(inst.PRs, ref) < 0 {
			inst.mu.Unlock()
			return false, nil
		}
		before := activePRNumber(inst)
		for idx := range inst.PRs {
			inst.PRs[idx].Pinned = pinned && inst.PRs[idx].Ref == ref
		}
		publishPRSetLocked(inst, before)
		return true, nil
	})
	return found
}

// DetachPR removes ref from the in-memory set and reports whether it was there.
// A detach of the unrepresented legacy number clears LegacyPRUnrepresented: the
// caller's tombstone now covers it.
func (i *Instance) DetachPR(ref domain.PRRef) bool {
	found, _ := sendSyncValue(i, func(s *instanceState) (bool, error) {
		return detachPRLocked(s.inst, ref), nil
	})
	return found
}

func detachPRLocked(inst *Instance, ref domain.PRRef) bool {
	inst.mu.Lock()
	idx := indexOfPR(inst.PRs, ref)
	if idx < 0 {
		inst.mu.Unlock()
		return false
	}
	before := activePRNumber(inst)
	inst.PRs = append(append([]domain.PRView(nil), inst.PRs[:idx]...), inst.PRs[idx+1:]...)
	inst.HasPRLinks = len(inst.PRs) > 0
	if inst.LegacyPRUnrepresented && inst.GitHubPRNumber == ref.Number() {
		inst.LegacyPRUnrepresented = false
	}
	if len(inst.PRs) == 0 && !inst.LegacyPRUnrepresented {
		// Removing the last link leaves nothing to project; PersistPRMirror clears the row the same way.
		inst.GitHubPRNumber, inst.GitHubPRURL, inst.GitHubPRStatusTerminal = 0, "", false
	}
	publishPRSetLocked(inst, before)
	return true
}

func (i *Instance) mutatePR(ref domain.PRRef, mutate func(*domain.PRView)) bool {
	found, _ := sendSyncValue(i, func(s *instanceState) (bool, error) {
		inst := s.inst
		inst.mu.Lock()
		idx := indexOfPR(inst.PRs, ref)
		if idx < 0 {
			inst.mu.Unlock()
			return false, nil
		}
		before := activePRNumber(inst)
		mutate(&inst.PRs[idx])
		publishPRSetLocked(inst, before)
		return true, nil
	})
	return found
}

// publishPRSetLocked mirrors the projection into the raw legacy fields, logs an
// active-PR change, then unlocks and publishes the snapshot. The caller holds inst.mu.
func publishPRSetLocked(inst *Instance, before int) {
	if m, ok := ProjectLegacyPRMirror(inst.PRs, inst.Branch); ok {
		inst.GitHubPRNumber, inst.GitHubPRURL, inst.GitHubPRStatusTerminal = m.Number, m.URL, m.Terminal
	}
	after, title := inst.GitHubPRNumber, inst.Title
	snap := buildSnapshot(inst)
	inst.mu.Unlock()
	inst.snapshot.Store(snap)
	if after != before {
		log.Info("session active pr changed", "session", title, "from", before, "to", after)
	}
}

// activePRNumber is the number the legacy scalars show now. The caller holds inst.mu.
func activePRNumber(inst *Instance) int {
	if m, ok := ProjectLegacyPRMirror(inst.PRs, inst.Branch); ok {
		return m.Number
	}
	return inst.GitHubPRNumber
}

func indexOfPR(prs []domain.PRView, ref domain.PRRef) int {
	for idx := range prs {
		if prs[idx].Ref == ref {
			return idx
		}
	}
	return -1
}

func FindPR(prs []domain.PRView, ref domain.PRRef) (domain.PRView, bool) {
	if idx := indexOfPR(prs, ref); idx >= 0 {
		return prs[idx], true
	}
	return domain.PRView{}, false
}

// persistPRAttach writes the new link and the mirror in one repository
// transaction. Persists of one session are serialized and read Snapshot() inside
// the lock, so a slower writer can never land an older mirror after a newer one.
// Crash-loss window: between the actor call returning and this commit the link
// exists only in memory; the next full save re-writes it (prPersistPending).
func (i *Instance) persistPRAttach(view domain.PRView) {
	i.prPersistMu.Lock()
	defer i.prPersistMu.Unlock()
	if i.prPersister == nil {
		i.prPersistPending.Store(true)
		return
	}
	snap := i.Snapshot()
	link := PRLink{Role: view.Role, Source: view.AttachSource, AttachedAt: view.AttachedAt, Initial: prInitialOf(view)}
	res, err := i.prPersister.AttachSessionPR(context.Background(), snap.UUID, view.Ref, link, prMirrorOf(snap.GitHub))
	switch {
	case err == nil && res.Tombstoned:
		_ = i.sendSyncErr(func(s *instanceState) error { detachPRLocked(s.inst, view.Ref); return nil })
		log.Info("attach skipped: PR was detached earlier", "session", snap.Title, "pr", view.Ref.String())
		// The tombstoned attach wrote no mirror, but the detach may have changed the
		// in-memory scalars (down to zero); the stored columns must follow.
		g := i.Snapshot().GitHub
		if perr := i.prPersister.PersistPRMirror(snap.UUID, g.GitHubPRURL, g.GitHubPRNumber, g.GitHubPRStatusTerminal); perr != nil {
			i.prPersistPending.Store(true)
			log.Warn("persist mirror after tombstoned attach failed", "session", snap.Title, "err", perr)
		}
	case err == nil:
	case errors.Is(err, ErrSessionNotPersisted):
		i.prPersistPending.Store(true)
	default:
		i.prPersistPending.Store(true)
		log.Warn("persist PR link failed; the next save retries", "session", snap.Title, "pr", view.Ref.String(), "err", err)
	}
}

func prInitialOf(v domain.PRView) PRInitial {
	return PRInitial{
		State: v.State, Terminal: v.StoredTerminal, URL: v.URL, Title: v.Title,
		HeadBranch: v.HeadBranch, BaseBranch: v.BaseBranch, IsDraft: v.IsDraft,
	}
}

// attachLegacyPR routes a legacy scalar write through AttachPR and reports
// whether it did. It declines (false) when there is no number, no resolved
// owner/repo, or no uuid, so the caller writes the scalar alone.
func (i *Instance) attachLegacyPR(number int, source domain.AttachSource, initial PRInitial) bool {
	if number <= 0 {
		return false
	}
	g := i.Snapshot().GitHub
	ref, err := domain.NewPRRef(g.GitHubHost, g.GitHubOwner, g.GitHubRepo, number)
	if err != nil {
		return false
	}
	switch err := i.AttachPR(ref, source, initial); {
	case err == nil:
		return true
	case errors.Is(err, ErrNoSessionUUID):
		if i.prLegacyPathLogged.CompareAndSwap(false, true) {
			log.Info("session has no uuid; writing the legacy PR scalar only", "session", i.Snapshot().Title)
		}
		return false
	default:
		log.Warn("attach PR failed; writing the legacy PR scalar only", "session", i.Snapshot().Title, "err", err)
		return false
	}
}

// AttachScannedPR links a PR the terminal scan found in push or `gh pr create`
// output. The caller has already matched the URL's host, owner and repo to the
// session's. A session that cannot hold a link (no uuid) takes the legacy scalar
// write instead, clearing the stored terminal flag when the number changes so a
// merged PR's true does not ride onto the new one.
func (i *Instance) AttachScannedPR(prURL string, number int) {
	if i.attachLegacyPR(number, domain.AttachSourcePushScan, PRInitial{URL: prURL}) {
		return
	}
	changed, _ := sendSyncValue(i, func(s *instanceState) (bool, error) {
		inst := s.inst
		inst.mu.Lock()
		changed := inst.GitHubPRNumber != number
		inst.GitHubPRURL, inst.GitHubPRNumber = prURL, number
		if changed {
			inst.GitHubPRStatusTerminal = false
		}
		snap := buildSnapshot(inst)
		inst.mu.Unlock()
		inst.snapshot.Store(snap)
		return changed, nil
	})
	if changed {
		i.persistTerminalClear()
	}
}

// persistTerminalClear writes terminal=false to the session row at once (title
// keyed, so it works without a uuid or a link). When that is not possible the
// terminalClearPending bit makes the next full save write it.
func (i *Instance) persistTerminalClear() {
	i.prPersistMu.Lock()
	defer i.prPersistMu.Unlock()
	if i.prPersister == nil {
		i.terminalClearPending.Store(true)
		return
	}
	title := i.Snapshot().Title
	if err := i.prPersister.UpdateGitHubPRStatusTerminal(title, false); err != nil {
		i.terminalClearPending.Store(true)
		log.Warn("clear stored PR terminal flag failed; the next save retries", "session", title, "err", err)
	}
}

// mirrorOutcome is the result of reconciling one session's legacy PR scalars.
type mirrorOutcome int

const (
	mirrorInSync mirrorOutcome = iota
	mirrorCorrected
	mirrorUnrepresentedSkipped
)

// prMirrorPersister is the storage seam the reconciler persists through.
type prMirrorPersister interface {
	PersistPRMirror(sessionUUID, url string, number int, terminal bool) error
}

// ReconcilePRMirror makes the legacy scalars equal the ActivePR projection. The
// in-memory fields are fixed through the actor, silently; the outcome is drift
// only when the stored columns (stored, nil when unknown) differ from the
// projection, or, without a stored value, when the in-memory fields did. A session
// whose legacy number is unrepresented and untombstoned is left alone (counted by
// the caller). A stored URL that is merely empty is not drift.
func (i *Instance) ReconcilePRMirror(stored *PRMirror, p prMirrorPersister) mirrorOutcome {
	out, _ := sendSyncValue(i, func(s *instanceState) (mirrorOutcome, error) {
		return reconcilePRMirrorLocked(s.inst, stored), nil
	})
	if out == mirrorCorrected {
		i.persistPRMirrorFromSnapshot(p)
	}
	return out
}

func reconcilePRMirrorLocked(inst *Instance, stored *PRMirror) mirrorOutcome {
	inst.mu.Lock()
	if inst.LegacyPRUnrepresented && inst.GitHubPRNumber > 0 {
		inst.mu.Unlock()
		return mirrorUnrepresentedSkipped
	}
	m, ok := ProjectLegacyPRMirror(inst.PRs, inst.Branch)
	if !ok {
		inst.mu.Unlock()
		return mirrorInSync
	}
	want := PRMirror{URL: m.URL, Number: m.Number, Terminal: m.Terminal}
	rawDrift := (PRMirror{URL: inst.GitHubPRURL, Number: inst.GitHubPRNumber, Terminal: inst.GitHubPRStatusTerminal}) != want
	drift := rawDrift
	if stored != nil {
		drift = mirrorDiffers(*stored, want)
	}
	if rawDrift {
		publishPRSetLocked(inst, inst.GitHubPRNumber)
	} else {
		inst.mu.Unlock()
	}
	if drift {
		return mirrorCorrected
	}
	return mirrorInSync
}

func mirrorDiffers(stored, want PRMirror) bool {
	if stored.Number != want.Number || stored.Terminal != want.Terminal {
		return true
	}
	return stored.URL != "" && stored.URL != want.URL
}

// persistPRMirrorFromSnapshot writes the snapshot's legacy scalars, serialized with
// AttachPR's persist so an older mirror never lands after a newer one.
func (i *Instance) persistPRMirrorFromSnapshot(p prMirrorPersister) {
	i.prPersistMu.Lock()
	defer i.prPersistMu.Unlock()
	snap := i.Snapshot()
	if snap.UUID == "" {
		return
	}
	g := snap.GitHub
	err := p.PersistPRMirror(snap.UUID, g.GitHubPRURL, g.GitHubPRNumber, g.GitHubPRStatusTerminal)
	if err != nil && !errors.Is(err, ErrSessionNotPersisted) {
		log.Warn("persist reconciled PR mirror failed; the next pass retries", "session", snap.Title, "err", err)
	}
}
