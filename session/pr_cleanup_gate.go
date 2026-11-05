package session

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// CleanupStatus is the result class of one terminal-item cleanup attempt.
type CleanupStatus int

const (
	// CleanupDone: nothing is left to clean (settle the item).
	CleanupDone CleanupStatus = iota
	// CleanupBlocked: the gate withheld worktree removal.
	CleanupBlocked
	// CleanupFailed: a removal failed; retry on the settle TTL.
	CleanupFailed
)

// CleanupOutcome is what WorktreeCleaner.CleanupTerminalItem reports.
type CleanupOutcome struct {
	Status CleanupStatus
	Reason string
	// LiveOnly marks a Blocked outcome held ONLY by a live work session. Those are
	// never settled (the sweep retries them every pass); a PR-held block settles for
	// the TTL because the terminal listener releases it promptly (review C7).
	LiveOnly bool
}

// Cleanup path labels of ssq_cleanup_gate_blocked_total{path}; the removal choke
// point reads the caller's label from the context (WithCleanupPath).
const (
	CleanupPathArchiveRPC = "archive_rpc"
	CleanupPathTransition = "transition"
	CleanupPathHook       = "hook"
	CleanupPathSweep      = "sweep"
	CleanupPathRework     = "rework"
	CleanupPathTombstone  = "tombstone"
	CleanupPathListener   = "listener"
	CleanupPathDetach     = "detach"
	cleanupPathUnknown    = "unknown"
)

type cleanupPathKey struct{}

// WithCleanupPath labels ctx with the removal path for the blocked metrics.
func WithCleanupPath(ctx context.Context, path string) context.Context {
	return context.WithValue(ctx, cleanupPathKey{}, path)
}

// CleanupPathFrom returns the label set by WithCleanupPath, or "unknown".
func CleanupPathFrom(ctx context.Context) string {
	if p, ok := ctx.Value(cleanupPathKey{}).(string); ok && p != "" {
		return p
	}
	return cleanupPathUnknown
}

// CleanupDoneOutcome is the all-clear outcome.
func CleanupDoneOutcome() CleanupOutcome { return CleanupOutcome{Status: CleanupDone} }

// CleanupBlockedOutcome wraps a gate verdict.
func CleanupBlockedOutcome(b CleanupBlock) CleanupOutcome {
	return CleanupOutcome{Status: CleanupBlocked, Reason: b.Reason, LiveOnly: b.LiveOnly}
}

// CleanupBlock is the gate's reason for withholding removal.
type CleanupBlock struct {
	Reason   string
	LiveOnly bool
}

// GateOptions tune one Blocked call.
type GateOptions struct {
	// IgnoreLive drops the live-work clause: used by the rework path, where the
	// just-spawned current-round session is live by construction and only PR
	// clauses may hold the prior rounds' worktrees.
	IgnoreLive bool
}

// CleanupGateSource is the storage slice the gate reads; *Storage implements it.
type CleanupGateSource interface {
	ListSessionPRViewsByUUIDs(ctx context.Context, uuids []string) (map[string][]PRView, error)
	ListItemGuardViewsByItemIDs(ctx context.Context, itemIDs []string) (map[string]ItemGuardView, error)
	FindInstanceDataByID(id string) (*InstanceData, error)
}

// PRCleanupGate decides whether a backlog-driven path may remove an item's
// worktrees. It is the single predicate behind cleanupItemWorktreesExcept: blocked
// while a PR tied to the worktree is not terminal (PR clauses, never flag-gated, they
// only withhold deletion) or while the current-round work session is live (live
// clause, only with the multi-PR scope on and a probe wired).
type PRCleanupGate struct {
	src         CleanupGateSource
	probe       LiveWorkProbe
	liveEnabled func() bool
	now         func() time.Time
}

// NewPRCleanupGate builds the gate. A nil probe or a false liveEnabled disables
// the live clause (today's behavior).
func NewPRCleanupGate(src CleanupGateSource, probe LiveWorkProbe, liveEnabled func() bool) *PRCleanupGate {
	return &PRCleanupGate{src: src, probe: probe, liveEnabled: liveEnabled, now: time.Now}
}

// Probe returns the live-work probe the gate holds (for the wiring assertion).
func (g *PRCleanupGate) Probe() LiveWorkProbe { return g.probe }

// Blocked reports whether removal must be withheld for sessions, with the reason
// naming each holding owner. Any lookup error blocks (fail closed).
func (g *PRCleanupGate) Blocked(ctx context.Context, sessions []ItemSessionSummary, opts GateOptions) (CleanupBlock, bool) {
	if reasons := g.prReasons(ctx, sessions); len(reasons) > 0 {
		return CleanupBlock{Reason: strings.Join(reasons, "; ")}, true
	}
	if opts.IgnoreLive || g.probe == nil || g.liveEnabled == nil || !g.liveEnabled() {
		return CleanupBlock{}, false
	}
	for _, s := range currentRoundWorkSessions(sessions) {
		live, err := g.probe.IsLive(ctx, s.SessionUUID, g.now())
		if err != nil {
			return CleanupBlock{Reason: fmt.Sprintf("session %s: live-work probe failed: %v", s.SessionUUID, err), LiveOnly: true}, true
		}
		if live {
			return CleanupBlock{Reason: fmt.Sprintf("session %s: still active", s.SessionUUID), LiveOnly: true}, true
		}
	}
	return CleanupBlock{}, false
}

func (g *PRCleanupGate) prReasons(ctx context.Context, sessions []ItemSessionSummary) []string {
	var uuids []string
	itemIDs := map[string]struct{}{}
	for _, s := range sessions {
		if s.SessionUUID == "" || s.Role != SessionRoleWork {
			continue
		}
		uuids = append(uuids, s.SessionUUID)
		if s.BacklogItemID != "" {
			itemIDs[s.BacklogItemID] = struct{}{}
		}
	}
	if len(uuids) == 0 {
		return nil
	}
	views, err := g.src.ListSessionPRViewsByUUIDs(ctx, uuids)
	if err != nil {
		return []string{"PR links unreadable: " + err.Error()}
	}
	var reasons []string
	for _, id := range uuids {
		d, derr := g.src.FindInstanceDataByID(id)
		if errors.Is(derr, ErrInstanceDataNotFound) {
			d = &InstanceData{UUID: id}
		} else if derr != nil {
			return append(reasons, fmt.Sprintf("session %s: unreadable: %v", id, derr))
		}
		if r := OwnPRsBlockReason(*d, views[id]); r != "" {
			reasons = append(reasons, fmt.Sprintf("session %s: %s", id, r))
		}
	}
	ids := make([]string, 0, len(itemIDs))
	for id := range itemIDs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	guards, err := g.src.ListItemGuardViewsByItemIDs(ctx, ids)
	if err != nil {
		return append(reasons, "item PR guards unreadable: "+err.Error())
	}
	for _, id := range ids {
		guard, ok := guards[id]
		if r := ItemGuardBlockReason(id, guard, ok); r != "" {
			reasons = append(reasons, r)
		}
	}
	return reasons
}

// OwnPRsBlockReason is the ordered PR decision for one session's own links (first
// match wins); "" means they permit deleting its worktree. rows are its link views.
// Shared by the cleanup gate and the retention sweeper so they cannot disagree.
func OwnPRsBlockReason(d InstanceData, rows []PRView) string {
	switch {
	case len(rows) == 0 && d.GitHubPRNumber == 0:
		return ""
	case len(rows) == 0:
		// A legacy number with no link is decided by the persisted flag alone,
		// never by the empty row set.
		if LegacyOnlyPRSafe(d) {
			return ""
		}
		return "legacy PR number with no link and no terminal flag"
	}
	if !PRsAllTerminal(rows, d.GitHubPRNumber) {
		return "a linked PR is open, unknown or unverified"
	}
	if d.GitHubPRNumber > 0 && !LegacyNumberRepresented(d, rows) && !LegacyOnlyPRSafe(d) {
		return "legacy PR number not carried by the links and not terminal"
	}
	return ""
}

// ItemGuardBlockReason is the item-level counterpart: an item any of whose
// item-direct PRs is not terminal, whose scalar PR number no link represents, or
// that has no evidence at all (ok false) holds its sessions. "" means no hold.
func ItemGuardBlockReason(itemID string, g ItemGuardView, ok bool) string {
	switch {
	case !ok:
		return "no PR evidence for item " + itemID
	case g.LegacyUnrepresented:
		return "item " + itemID + " has a PR number no link represents"
	}
	for _, v := range g.Direct {
		if !v.Terminal() {
			return "item " + itemID + " has an item-direct PR that is not terminal"
		}
	}
	return ""
}

// OpenPRRefs returns the refs of the views that are not terminal (open, unknown or
// unverified), for the warnings on intentionally ungated removals.
func OpenPRRefs(views []PRView) []PRRef {
	var open []PRRef
	for _, v := range views {
		if !v.Terminal() {
			open = append(open, v.Ref)
		}
	}
	return open
}

// OpenPRsOfSession lists the open or unknown PR refs a session links, read-only,
// for the warnings on intentionally ungated removals (T5.4.5). Never blocks.
func (g *PRCleanupGate) OpenPRsOfSession(ctx context.Context, sessionUUID string) ([]PRRef, error) {
	views, err := g.src.ListSessionPRViewsByUUIDs(ctx, []string{sessionUUID})
	if err != nil {
		return nil, err
	}
	return OpenPRRefs(views[sessionUUID]), nil
}

// BlockedSet tracks which (path, item) pairs are blocked now, so the counter and the
// log line fire once per newly blocked pair and the gauge reports the current size.
type BlockedSet struct {
	mu      sync.Mutex
	entries map[string]map[string]blockedEntry // path -> item id -> entry
}

type blockedEntry struct {
	since  time.Time
	reason string
}

// NewBlockedSet returns an empty set.
func NewBlockedSet() *BlockedSet { return &BlockedSet{entries: map[string]map[string]blockedEntry{}} }

// Block records the pair and reports whether it is newly blocked.
func (b *BlockedSet) Block(path, itemID, reason string, now time.Time) (first bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.entries[path] == nil {
		b.entries[path] = map[string]blockedEntry{}
	}
	if cur, ok := b.entries[path][itemID]; ok {
		cur.reason = reason
		b.entries[path][itemID] = cur
		return false
	}
	b.entries[path][itemID] = blockedEntry{since: now, reason: reason}
	return true
}

// Unblock removes the pair from every path.
func (b *BlockedSet) Unblock(itemID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, m := range b.entries {
		delete(m, itemID)
	}
}

// Count is how many items are blocked on path now.
func (b *BlockedSet) Count(path string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.entries[path])
}

// Items lists every blocked item with the time it first blocked, oldest first.
func (b *BlockedSet) Items() []BlockedItem {
	b.mu.Lock()
	defer b.mu.Unlock()
	seen := map[string]BlockedItem{}
	for path, m := range b.entries {
		for id, e := range m {
			if cur, ok := seen[id]; !ok || e.since.Before(cur.Since) {
				seen[id] = BlockedItem{ItemID: id, Path: path, Since: e.since, Reason: e.reason}
			}
		}
	}
	out := make([]BlockedItem, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Since.Before(out[j].Since) })
	return out
}

// BlockedItem is one entry of the blocked set.
type BlockedItem struct {
	ItemID string
	Path   string
	Reason string // names the holding owners
	Since  time.Time
}
