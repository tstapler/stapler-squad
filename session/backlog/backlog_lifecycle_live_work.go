package backlog

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// liveWorkState is the listener's live-work wiring and per-item hold bookkeeping.
type liveWorkState struct {
	mu    sync.Mutex
	probe session.LiveWorkProbe
	clock func() time.Time // nil: time.Now

	refresher PRRowRefresher

	firstHeld map[string]time.Time // item id -> first time the hold was observed
	notified  map[string]bool      // item id -> max-hold notification already sent
}

// SetLiveWorkProbe wires the shared live-work probe. Nil (or the reader:backlog
// scope off) means today's behavior: no hold.
func (l *BacklogLifecycleListener) SetLiveWorkProbe(p session.LiveWorkProbe) {
	l.liveWork.mu.Lock()
	defer l.liveWork.mu.Unlock()
	l.liveWork.probe = p
}

// LiveWorkProbe returns the wired probe (nil when unwired); the wiring assertion
// compares it with the instances held by BacklogService and the gate.
func (l *BacklogLifecycleListener) LiveWorkProbe() session.LiveWorkProbe {
	l.liveWork.mu.Lock()
	defer l.liveWork.mu.Unlock()
	return l.liveWork.probe
}

func (l *BacklogLifecycleListener) liveWorkNow() time.Time {
	l.liveWork.mu.Lock()
	defer l.liveWork.mu.Unlock()
	if l.liveWork.clock != nil {
		return l.liveWork.clock()
	}
	return time.Now()
}

// heldByLiveWork decides the live-work hold for an item whose PR verdict is Done.
// held and maxHoldExpired are mutually exclusive: held means leave the item as it
// is and re-evaluate next pass; maxHoldExpired means the session is still live but
// the hold outlasted LiveWorkMaxHold since the FIRST merge, so the caller proceeds
// with a warning (expiry wins over a probe error). Off unless reader:backlog is on
// and a probe is wired. A probe error is treated as live; the real probe caps that
// itself.
func (l *BacklogLifecycleListener) heldByLiveWork(ctx context.Context, itemID string, prs []session.PRView) (held, maxHoldExpired bool) {
	probe := l.LiveWorkProbe()
	if probe == nil || !l.backlogReaderEnabled() {
		return false, false
	}
	sessions, err := l.storage.ListItemSessions(ctx, itemID)
	if err != nil {
		log.WarningLog().Printf("[LiveWork] item=%s ListItemSessions: %v (held)", itemID, err)
		return true, false
	}
	now := l.liveWorkNow()
	if !l.anyLive(ctx, probe, itemID, now, session.CurrentRoundWorkSessions(sessions)) {
		l.clearLiveWorkHold(itemID)
		return false, false
	}
	start := session.LiveWorkHoldStart(prs, l.firstHeldAt(itemID, now))
	if session.LiveWorkHoldExpired(now, start, session.LiveWorkMaxHold) {
		return false, true
	}
	session.CountLiveWorkResult(session.LiveWorkResultHeld)
	return true, false
}

func (l *BacklogLifecycleListener) anyLive(ctx context.Context, probe session.LiveWorkProbe, itemID string, now time.Time, work []backlogdto.ItemSessionSummary) bool {
	for _, s := range work {
		live, err := probe.IsLive(ctx, s.SessionUUID, now)
		if err != nil {
			log.WarningLog().Printf("[LiveWork] item=%s session=%s probe error, held: %v", itemID, s.SessionUUID, err)
			return true
		}
		if live {
			return true
		}
	}
	return false
}

func (l *BacklogLifecycleListener) firstHeldAt(itemID string, now time.Time) time.Time {
	l.liveWork.mu.Lock()
	defer l.liveWork.mu.Unlock()
	if l.liveWork.firstHeld == nil {
		l.liveWork.firstHeld = map[string]time.Time{}
	}
	if at, ok := l.liveWork.firstHeld[itemID]; ok {
		return at
	}
	l.liveWork.firstHeld[itemID] = now
	return now
}

func (l *BacklogLifecycleListener) clearLiveWorkHold(itemID string) {
	l.liveWork.mu.Lock()
	defer l.liveWork.mu.Unlock()
	delete(l.liveWork.firstHeld, itemID)
	delete(l.liveWork.notified, itemID)
}

// holdMergedItemForLiveWork is the shared merge-to-done guard of ReconcilePRPending
// and reconcileBouncingItems. It reports true when the caller must leave the item
// alone this pass. On max-hold expiry it logs a warning, counts it and sends one
// notification, then lets the transition run; the live session itself stays
// protected by RULE L.
func (l *BacklogLifecycleListener) holdMergedItemForLiveWork(ctx context.Context, er *session.EntRepository, itemID, title string) bool {
	if l.LiveWorkProbe() == nil || !l.backlogReaderEnabled() {
		return false
	}
	prs, err := er.ItemPRsByID(ctx, itemID)
	if err != nil {
		log.WarningLog().Printf("[LiveWork] item=%s pr set unreadable, held: %v", itemID, err)
		return true
	}
	held, expired := l.heldByLiveWork(ctx, itemID, prs)
	if held {
		log.InfoLog().Printf("[LiveWork] item=%s merged, session still active: held", itemID)
		return true
	}
	if expired {
		l.reportMaxHold(itemID, title)
	}
	return false
}

func (l *BacklogLifecycleListener) reportMaxHold(itemID, title string) {
	l.liveWork.mu.Lock()
	if l.liveWork.notified == nil {
		l.liveWork.notified = map[string]bool{}
	}
	first := !l.liveWork.notified[itemID]
	l.liveWork.notified[itemID] = true
	l.liveWork.mu.Unlock()
	if !first {
		return
	}
	session.CountLiveWorkResult(session.LiveWorkResultMaxHold)
	log.WarningLog().Printf("[LiveWork] item=%s still has a live session %s after its first merge: marking done; the live session keeps its pane and worktree", itemID, session.LiveWorkMaxHold)
	l.notify(itemID, "Session still active after merge",
		fmt.Sprintf("%s: a session was still active %s after the PR merged. The item is marked done; local files are kept until the session ends.", title, session.LiveWorkMaxHold),
		8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
		false, true)
}

// prRefreshWait bounds the awaited row refresh a reconciler triggers when it
// observes a merge (safe direction: on timeout the item moves on the next pass).
const prRefreshWait = 10 * time.Second

// PRRowRefresher refreshes one PR's state row from GitHub and returns once the poll
// core has written it. *PRStatusPoller implements it.
type PRRowRefresher interface {
	Refresh(ctx context.Context, ref session.PRRef, src session.PRObservationSource) error
}

// SetPRRowRefresher wires the poller's per-PR refresh into the reconcilers.
func (l *BacklogLifecycleListener) SetPRRowRefresher(r PRRowRefresher) {
	l.liveWork.mu.Lock()
	defer l.liveWork.mu.Unlock()
	l.liveWork.refresher = r
}

// itemVerdictAfterMerge is how ReconcilePRPending turns "IsPRMerged said yes" into a
// verdict without overlaying its own observation: it triggers an awaited refresh of
// the tracked PR's row, then reads the verdict from the rows only (the poll core is
// the single writer). A refresh that fails leaves the row as it was, so the verdict
// can only be later, never earlier.
func (l *BacklogLifecycleListener) itemVerdictAfterMerge(ctx context.Context, er *session.EntRepository, itemID, prURL string, prNumber int) (session.ItemVerdict, error) {
	l.liveWork.mu.Lock()
	refresher := l.liveWork.refresher
	l.liveWork.mu.Unlock()
	if refresher != nil {
		if ref, err := session.ParseItemPRRef(prURL, prNumber); err == nil {
			rctx, cancel := context.WithTimeout(ctx, prRefreshWait)
			if rerr := refresher.Refresh(rctx, ref, session.PRSourceReconcilerRefresh); rerr != nil {
				log.WarningLog().Printf("[BacklogLifecycle] item=%s refresh of merged PR %s: %v (verdict from existing rows)", itemID, ref, rerr)
			}
			cancel()
		}
	}
	prs, err := er.ItemPRsByID(ctx, itemID)
	if err != nil {
		return session.ItemVerdictOpen, err
	}
	return session.ItemPRVerdict(prs), nil
}

// ruleLProbe is the probe RULE L consults: nil (skip nothing, today's behavior)
// unless reader:backlog is on and a probe is wired.
func (l *BacklogLifecycleListener) ruleLProbe() session.LiveWorkProbe {
	if !l.backlogReaderEnabled() {
		return nil
	}
	return l.LiveWorkProbe()
}
