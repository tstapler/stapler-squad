package session

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
)

// PRSetReader derives a backlog item's PR set at read time. The set is never
// stored: item-direct links plus the links of the item's current round, minus
// the item's own tombstones, plus (when nothing represents it) the legacy
// pr_number as an `unknown` PR.
type PRSetReader interface {
	ItemPRs(ctx context.Context, item *ent.BacklogItem) ([]domain.PRView, error)
}

// itemPRSource is the slice of the repository the derivation reads.
type itemPRSource interface {
	ListByItem(ctx context.Context, itemID string) ([]domain.PRView, error)
	ListBySessionUUIDs(ctx context.Context, uuids []string) (map[string][]domain.PRView, error)
	ListSessionTombstonesByUUIDs(ctx context.Context, uuids []string) (map[string][]domain.PRRef, error)
	ListItemTombstones(ctx context.Context, itemIDs []string) (map[string][]domain.PRRef, error)
	ListItemSessions(ctx context.Context, itemID string) ([]backlogdto.ItemSessionSummary, error)
}

var _ PRSetReader = (*EntRepository)(nil)

// ItemPRs implements PRSetReader over the repository.
func (r *EntRepository) ItemPRs(ctx context.Context, item *ent.BacklogItem) ([]domain.PRView, error) {
	return deriveItemPRs(ctx, r, item.ID.String(), item.PrURL, item.PrNumber)
}

// ItemPRsByID is ItemPRs for callers holding only the item id (string UUID).
func (r *EntRepository) ItemPRsByID(ctx context.Context, itemID string) ([]domain.PRView, error) {
	id, err := uuid.Parse(itemID)
	if err != nil {
		return nil, fmt.Errorf("item prs: invalid item id %q: %w", itemID, err)
	}
	item, err := r.Client.BacklogItem.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("item prs: load item %s: %w", itemID, err)
	}
	return r.ItemPRs(ctx, item)
}

// workSessionNewer orders work sessions newest first: created_at, then the larger id.
func workSessionNewer(a, b backlogdto.ItemSessionSummary) bool {
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.After(b.CreatedAt)
	}
	return a.ID > b.ID
}

// currentRoundSession walks work sessions newest first. A session with neither
// link nor tombstone is skipped (skipped reports whether any was); the first one
// with a link, or with a tombstone and no link, is the round. "" means none.
// ended_at is deliberately never read.
func currentRoundSession(work []backlogdto.ItemSessionSummary, links map[string][]domain.PRView, tombs map[string][]domain.PRRef) (uuid string, skipped bool) {
	sorted := append([]backlogdto.ItemSessionSummary(nil), work...)
	sort.Slice(sorted, func(i, j int) bool { return workSessionNewer(sorted[i], sorted[j]) })
	for _, s := range sorted {
		if len(links[s.SessionUUID]) > 0 || len(tombs[s.SessionUUID]) > 0 {
			return s.SessionUUID, skipped
		}
		skipped = true
	}
	return "", skipped
}

func workSessions(all []backlogdto.ItemSessionSummary) (work []backlogdto.ItemSessionSummary, uuids []string) {
	for _, s := range all {
		if s.Role == SessionRoleWork && s.SessionUUID != "" {
			work = append(work, s)
			uuids = append(uuids, s.SessionUUID)
		}
	}
	return work, uuids
}

func deriveItemPRs(ctx context.Context, src itemPRSource, itemID, prURL string, prNumber int) ([]domain.PRView, error) {
	direct, err := src.ListByItem(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("item prs: list item links: %w", err)
	}
	itemTombs, err := src.ListItemTombstones(ctx, []string{itemID})
	if err != nil {
		return nil, fmt.Errorf("item prs: list item tombstones: %w", err)
	}
	sessions, err := src.ListItemSessions(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("item prs: list item sessions: %w", err)
	}
	work, uuids := workSessions(sessions)

	var roundLinks []domain.PRView
	if len(uuids) > 0 {
		links, lerr := src.ListBySessionUUIDs(ctx, uuids)
		if lerr != nil {
			return nil, fmt.Errorf("item prs: list work session links: %w", lerr)
		}
		tombs, terr := src.ListSessionTombstonesByUUIDs(ctx, uuids)
		if terr != nil {
			return nil, fmt.Errorf("item prs: list work session tombstones: %w", terr)
		}
		round, skipped := currentRoundSession(work, links, tombs)
		if skipped {
			log.InfoLog().Printf("[ItemPRs] item %s current round fell back to earlier session", itemID)
		}
		roundLinks = links[round]
	}

	set := domain.DistinctPRs(subtractRefs(append(append([]domain.PRView(nil), direct...), roundLinks...), itemTombs[itemID]))
	return appendLegacyFallback(ctx, src, set, sessions, itemPRInputs{itemID, prURL, prNumber, direct, itemTombs[itemID]})
}

func subtractRefs(prs []domain.PRView, drop []domain.PRRef) []domain.PRView {
	if len(drop) == 0 {
		return prs
	}
	out := prs[:0:0]
	for _, v := range prs {
		if !containsRef(drop, v.Ref) {
			out = append(out, v)
		}
	}
	return out
}

// appendLegacyFallback adds one `unknown` PR for pr_number when no link of the
// item or any of its sessions represents it and no tombstone of the item or any
// session covers it. It returns before the all-rounds query when the derived set
// already holds the number.
func appendLegacyFallback(ctx context.Context, src itemPRSource, set []domain.PRView, sessions []backlogdto.ItemSessionSummary, in itemPRInputs) ([]domain.PRView, error) {
	prURL, prNumber := in.prURL, in.prNumber
	if prNumber <= 0 {
		return set, nil
	}
	ref, perr := ParseItemPRRef(prURL, prNumber)
	if perr != nil {
		return set, nil // unbuildable ref: the cleanup gate fails closed on it, not the set
	}
	for _, v := range set {
		if v.Ref == ref {
			return set, nil
		}
	}
	cov, err := loadSessionCoverage(ctx, src, sessions, in)
	if err != nil {
		return nil, err
	}
	if cov.Represented(ref) || cov.CoveredByTombstone(ref) {
		return set, nil
	}
	return append(set, domain.PRView{
		Ref: ref, State: domain.PRStateUnknown, URL: strings.TrimSpace(prURL),
		BacklogItemID: in.itemID, Role: domain.PRRolePrimary, AttachSource: domain.AttachSourceBackfill,
	}), nil
}

// itemPRInputs bundles what deriveItemPRs already read so the fallback does not re-query it.
type itemPRInputs struct {
	itemID    string
	prURL     string
	prNumber  int
	direct    []domain.PRView
	itemTombs []domain.PRRef
}

func loadSessionCoverage(ctx context.Context, src itemPRSource, sessions []backlogdto.ItemSessionSummary, in itemPRInputs) (ItemPRCoverage, error) {
	cov := ItemPRCoverage{
		ItemTombstones: in.itemTombs,
		SessionLinks:   map[string][]domain.PRRef{}, SessionTombstones: map[string][]domain.PRRef{},
	}
	for _, v := range in.direct {
		cov.ItemLinks = append(cov.ItemLinks, v.Ref)
	}
	var uuids []string
	for _, s := range sessions {
		if s.SessionUUID != "" {
			uuids = append(uuids, s.SessionUUID)
		}
	}
	if len(uuids) == 0 {
		return cov, nil
	}
	links, err := src.ListBySessionUUIDs(ctx, uuids)
	if err != nil {
		return cov, fmt.Errorf("item prs: coverage session links: %w", err)
	}
	for u, views := range links {
		for _, v := range views {
			cov.SessionLinks[u] = append(cov.SessionLinks[u], v.Ref)
		}
	}
	tombs, err := src.ListSessionTombstonesByUUIDs(ctx, uuids)
	if err != nil {
		return cov, fmt.Errorf("item prs: coverage session tombstones: %w", err)
	}
	cov.SessionTombstones = tombs
	return cov, nil
}
