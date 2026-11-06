package services

import (
	"context"
	"sort"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/adapters"
	"github.com/tstapler/stapler-squad/session"
)

// attachPRSet fills the item detail's pull_requests (the derived set) and pr_holds
// (why cleanup is held). Detail responses only: list responses never carry holds (K11).
// Off unless reader:backlog is on, so flag off the fields stay empty.
func (s *BacklogService) attachPRSet(ctx context.Context, p *sessionv1.BacklogItem, item *session.BacklogItemData) {
	if !s.cleanup.scopeOn() {
		return
	}
	prs, err := s.storage.ItemPRs(ctx, item.ID)
	if err != nil {
		log.WarningLog().Printf("[GetBacklogItem] item PR set unreadable for %s: %v", item.ID, err)
		return
	}
	p.PullRequests = adapters.SessionPullRequests(prs, "")
	p.PrHolds = s.prHolds(ctx, item, prs)
}

// populatePRCounts fills each stuck row's pr_count / pr_merged_count from the
// item's derived PR set. It is a no-op unless reader:backlog is on, and a failed
// read leaves that row's counts at 0 ("not computed").
func (s *BacklogService) populatePRCounts(ctx context.Context, items []*sessionv1.StuckBacklogItem) {
	if !s.cleanup.scopeOn() {
		return
	}
	for _, it := range items {
		prs, err := s.storage.ItemPRs(ctx, it.ItemId)
		if err != nil {
			log.WarningLog().Printf("[ListStuckBacklogItems] item PR set unreadable for %s: %v", it.ItemId, err)
			continue
		}
		it.PrCount, it.PrMergedCount = countItemPRs(prs)
	}
}

// countItemPRs returns the set size and how many of its PRs are merged.
func countItemPRs(prs []session.PRView) (total, merged int32) {
	for _, v := range prs {
		total++
		if v.State == session.PRStateMerged {
			merged++
		}
	}
	return total, merged
}

// prHolds derives the held lines from the same facts the gate reads: non-terminal PRs
// of the derived set, superseded rounds' non-terminal PRs, a live work session, and a
// live session past the max hold. The client never recomputes a hold.
func (s *BacklogService) prHolds(ctx context.Context, item *session.BacklogItemData, prs []session.PRView) []*sessionv1.PrHold {
	holds := nonTerminalPRHolds(prs, "")
	inSet := map[session.PRRef]bool{}
	for _, v := range prs {
		inSet[v.Ref] = true
	}
	sessions, err := s.storage.ListItemSessions(ctx, item.ID)
	if err != nil {
		return holds
	}
	holds = append(holds, s.supersededRoundHolds(ctx, sessions, inSet)...)
	return append(holds, s.liveWorkHolds(ctx, sessions, prs)...)
}

// nonTerminalPRHolds is one hold per PR that keeps its owner's worktree. ownerOverride,
// when set, names the owning session for every hold; otherwise each view's own
// SessionUUID is used.
func nonTerminalPRHolds(prs []session.PRView, ownerOverride string) []*sessionv1.PrHold {
	var holds []*sessionv1.PrHold
	for _, v := range prs {
		kind, held := holdKindOf(v)
		if !held {
			continue
		}
		owner := v.SessionUUID
		if ownerOverride != "" {
			owner = ownerOverride
		}
		holds = append(holds, &sessionv1.PrHold{Kind: kind, Pr: prRefToProto(v.Ref), OwnerSessionId: owner})
	}
	return holds
}

func holdKindOf(v session.PRView) (sessionv1.PrHold_Kind, bool) {
	switch {
	case v.Terminal():
		return 0, false
	case v.State == session.PRStateUnknown && v.StoredTerminal:
		return sessionv1.PrHold_KIND_UNVERIFIED, true
	case v.State == session.PRStateUnknown:
		return sessionv1.PrHold_KIND_UNKNOWN, true
	default:
		return sessionv1.PrHold_KIND_OPEN_PR, true
	}
}

// supersededRoundHolds names non-terminal PRs only an earlier round's session links:
// the verdict ignores them but they still hold that round's worktree.
func (s *BacklogService) supersededRoundHolds(ctx context.Context, sessions []session.ItemSessionSummary, inSet map[session.PRRef]bool) []*sessionv1.PrHold {
	var uuids []string
	for _, is := range sessions {
		if is.Role == session.SessionRoleWork && is.SessionUUID != "" {
			uuids = append(uuids, is.SessionUUID)
		}
	}
	views, err := s.storage.ListSessionPRViewsByUUIDs(ctx, uuids)
	if err != nil {
		return nil
	}
	var holds []*sessionv1.PrHold
	for _, id := range uuids {
		for _, v := range views[id] {
			if !inSet[v.Ref] && !v.Terminal() {
				holds = append(holds, &sessionv1.PrHold{Kind: sessionv1.PrHold_KIND_SUPERSEDED_ROUND, Pr: prRefToProto(v.Ref), OwnerSessionId: id})
			}
		}
	}
	sort.SliceStable(holds, func(i, j int) bool { return holds[i].OwnerSessionId < holds[j].OwnerSessionId })
	return holds
}

// liveWorkHolds reports SESSION_ACTIVE while the newest work session is live, and
// MAX_HOLD instead once the first merge is older than the max hold.
func (s *BacklogService) liveWorkHolds(ctx context.Context, sessions []session.ItemSessionSummary, prs []session.PRView) []*sessionv1.PrHold {
	probe := s.cleanup.ruleLProbe()
	if probe == nil {
		return nil
	}
	now := time.Now()
	for _, is := range session.NewestWorkSession(sessions) {
		live, err := probe.IsLive(ctx, is.SessionUUID, now)
		if err != nil || !live {
			continue
		}
		kind := sessionv1.PrHold_KIND_SESSION_ACTIVE
		if start := session.LiveWorkHoldStart(prs, time.Time{}); session.LiveWorkHoldExpired(now, start, session.LiveWorkMaxHold) {
			kind = sessionv1.PrHold_KIND_MAX_HOLD
		}
		return []*sessionv1.PrHold{{Kind: kind, OwnerSessionId: is.SessionUUID, IdleWindowSeconds: int32(session.LiveWorkIdleWindow / time.Second)}}
	}
	return nil
}

func prRefToProto(r session.PRRef) *sessionv1.PullRequestRef {
	return &sessionv1.PullRequestRef{Host: r.Host(), Owner: r.Owner(), Repo: r.Repo(), Number: int32(r.Number())} //nolint:gosec // PR numbers are far below int32 max
}
