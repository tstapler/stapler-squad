package backlog

import (
	"context"
	"sort"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// maxListenerCleanupsPerTick bounds how many done/archived items one terminal batch
// cleans; the rest wait for the 60 s reconcileTerminalItemWorktrees re-evaluation.
const maxListenerCleanupsPerTick = 20

// terminalWorkQueue is how many terminal batches may wait for the cleanup worker
// before new ones are dropped (the 60 s sweep still covers them).
const terminalWorkQueue = 64

// PRStatesWentTerminal implements PRTerminalListener: it enqueues the batch for a
// bounded worker and returns at once, so a slow worktree removal never stalls the
// poll core. A full queue drops the batch; the 60 s sweep re-evaluates every
// terminal item anyway.
func (l *BacklogLifecycleListener) PRStatesWentTerminal(stateIDs []int) {
	l.terminalWork.once.Do(func() {
		l.terminalWork.ch = make(chan []int, terminalWorkQueue)
		go l.runTerminalCleanups()
	})
	batch := append([]int(nil), stateIDs...)
	select {
	case l.terminalWork.ch <- batch:
	default:
		log.WarningLog().Printf("[PRTerminal] cleanup queue full, dropped %d state(s); the 60s sweep will re-evaluate", len(batch))
	}
}

func (l *BacklogLifecycleListener) runTerminalCleanups() {
	for {
		select {
		case <-l.shutdownCtx.Done():
			return
		case batch := <-l.terminalWork.ch:
			l.cleanupItemsOfTerminalStates(l.shutdownCtx, batch)
		}
	}
}

// cleanupItemsOfTerminalStates resolves the owners of the states' links to their
// items, keeps the done/archived ones and re-runs the (gated) cleanup for at most
// maxListenerCleanupsPerTick of them, FIFO. No revive or Start calls.
func (l *BacklogLifecycleListener) cleanupItemsOfTerminalStates(ctx context.Context, stateIDs []int) {
	if l.getWorktreeCleaner() == nil {
		return
	}
	sessionUUIDs, itemIDs, err := l.storage.ListLinkOwnersByStateIDs(ctx, stateIDs)
	if err != nil {
		log.WarningLog().Printf("[PRTerminal] resolving owners of %d state(s): %v", len(stateIDs), err)
		return
	}
	if len(sessionUUIDs) > 0 {
		byUUID, lerr := l.storage.ListItemIDsBySessionUUIDs(ctx, sessionUUIDs)
		if lerr != nil {
			log.WarningLog().Printf("[PRTerminal] resolving items of %d session(s): %v", len(sessionUUIDs), lerr)
			return
		}
		for _, ids := range byUUID {
			itemIDs = append(itemIDs, ids...)
		}
	}
	cleaned := 0
	for _, id := range distinctSorted(itemIDs) {
		if cleaned >= maxListenerCleanupsPerTick {
			return
		}
		item, gerr := l.storage.GetBacklogItem(ctx, id)
		if gerr != nil || !session.IsTerminalStatus(session.BacklogStatus(item.Status)) {
			continue
		}
		l.cleanupTerminalItemSync(session.WithCleanupPath(ctx, session.CleanupPathListener), id)
		cleaned++
	}
}

func distinctSorted(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
