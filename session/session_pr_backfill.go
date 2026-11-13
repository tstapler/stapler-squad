package session

// session_pr_backfill.go imports every legacy single-PR reference into
// PullRequestState + SessionPR rows. It has no run-once marker: representation
// by a link is the cursor, so each boot runs a bounded pass over the sessions
// and items whose legacy PR is not yet represented (never linked, or written by
// an older binary after a rollback). It is constructive-only and never revives
// anything. It is NOT a startupMigration: it runs only through
// Storage.RunPRStartupMaintenance (service bootstrap), see
// session_pr_maintenance.go.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/itemsession"
	entsession "github.com/tstapler/stapler-squad/session/ent/session"
)

const (
	// maxBackfillWritesPerBoot caps successful imports per candidate kind per boot;
	// the rest are deferred (they stay protected as unrepresented legacy numbers).
	maxBackfillWritesPerBoot = 2000
	// backfillLoggedTitles is how many skipped titles a boot logs.
	backfillLoggedTitles = 10
)

// ErrItemBadPRRef means an item's pr_url/pr_number do not yield a PRRef.
var ErrItemBadPRRef = errors.New("session: item pr_url does not yield a PR ref")

var errSessionNoRepo = errors.New("session has a PR number but no github owner/repo")

// ParseItemPRRef builds the PRRef of a backlog item from its pr_url and
// pr_number: path /{owner}/{repo}/pull/{n}, host from the URL (GHE included),
// and the number in the URL must equal pr_number. No git-remote fallback.
func ParseItemPRRef(prURL string, prNumber int) (domain.PRRef, error) {
	bad := func() (domain.PRRef, error) {
		return domain.PRRef{}, fmt.Errorf("%w: url %q number %d", ErrItemBadPRRef, prURL, prNumber)
	}
	u, err := url.Parse(strings.TrimSpace(prURL))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return bad()
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 || parts[2] != "pull" {
		return bad()
	}
	n, err := strconv.Atoi(parts[3])
	if err != nil || n != prNumber {
		return bad()
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	ref, err := domain.NewPRRef(host, parts[0], strings.TrimSuffix(parts[1], ".git"), n)
	if err != nil {
		return bad()
	}
	return ref, nil
}

// sessionLegacyRef builds the PRRef of a session's legacy columns. An empty
// github_host means github.com, though a GOPATH-style cloned path may still
// reveal a GHE host.
func sessionLegacyRef(owner, repo, host, path string, number int) (domain.PRRef, error) {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(repo) == "" {
		return domain.PRRef{}, errSessionNoRepo
	}
	if host == "" {
		if repoRef, err := github.NewRepoRef(owner, repo); err == nil {
			host = hostFromClonedPath(path, repoRef)
		}
	}
	return domain.NewPRRef(host, owner, repo, number)
}

func synthesizePRURL(ref domain.PRRef) string {
	return fmt.Sprintf("https://%s/%s/%s/pull/%d", ref.Host(), ref.Owner(), ref.Repo(), ref.Number())
}

// ItemPRCoverage is everything needed to decide whether an item's scalar PR is
// already represented by a link or covered by a detach tombstone.
type ItemPRCoverage struct {
	ItemLinks         []domain.PRRef            // item-direct links
	SessionLinks      map[string][]domain.PRRef // links of each of the item's sessions, every round and role
	ItemTombstones    []domain.PRRef
	SessionTombstones map[string][]domain.PRRef
}

func containsRef(refs []domain.PRRef, ref domain.PRRef) bool {
	for _, r := range refs {
		if r == ref {
			return true
		}
	}
	return false
}

// Represented reports whether any link of the item or of any of its sessions is ref.
func (c ItemPRCoverage) Represented(ref domain.PRRef) bool {
	if containsRef(c.ItemLinks, ref) {
		return true
	}
	for _, refs := range c.SessionLinks {
		if containsRef(refs, ref) {
			return true
		}
	}
	return false
}

// CoveredByTombstone is the wide tombstone test: a tombstone of the item OR of
// any of its sessions (every round and role), so detaching a session-only PR can
// never make it look "unrepresented" again.
func (c ItemPRCoverage) CoveredByTombstone(ref domain.PRRef) bool {
	if containsRef(c.ItemTombstones, ref) {
		return true
	}
	for _, refs := range c.SessionTombstones {
		if containsRef(refs, ref) {
			return true
		}
	}
	return false
}

// ItemHasUnrepresentedLegacyPR reports the item's scalar PR as a ref when it has a
// buildable one that no link represents and no tombstone covers; the error is
// ErrItemBadPRRef for the skip class (unbuildable ref).
func ItemHasUnrepresentedLegacyPR(prURL string, prNumber int, c ItemPRCoverage) (domain.PRRef, bool, error) {
	if prNumber <= 0 {
		return domain.PRRef{}, false, nil
	}
	ref, err := ParseItemPRRef(prURL, prNumber)
	if err != nil {
		return domain.PRRef{}, false, err
	}
	return ref, !c.Represented(ref) && !c.CoveredByTombstone(ref), nil
}

// PRBackfillReport counts what one backfill pass did.
type PRBackfillReport struct {
	CreatedSessionLinks   int
	CreatedItemLinks      int
	ImportedUnrepresented int // session imports into a session that already had links
	SkippedNoRepo         int // legacy number but no owner/repo: protected, not guessed
	SkippedNoUUID         int
	SkippedInvalid        int
	SkippedItemBadRef     int
	SkippedItemInvalid    int
	SkippedTombstoned     int
	Deferred              int
	SkippedTitles         []string // first backfillLoggedTitles
}

func (r *PRBackfillReport) skipTitle(title string) {
	if len(r.SkippedTitles) < backfillLoggedTitles {
		r.SkippedTitles = append(r.SkippedTitles, title)
	}
}

type prBackfillCandidate struct {
	owner    PROwner
	ref      domain.PRRef
	title    string
	url      string
	terminal bool // seeded as an unverified terminal row only if EVERY contributor says so
	isItem   bool
	hadLinks bool // session already had other links (a rollback import)
}

// prBackfillHook is a test seam called before each import; nil in production.
type prBackfillHook func(domain.PRRef) error

// runPRBackfill is one pass. A returned error is transient (retry); deterministic
// per-row failures are counted in the report and the pass continues.
func (r *EntRepository) runPRBackfill(ctx context.Context, hook prBackfillHook) (PRBackfillReport, error) {
	var rep PRBackfillReport
	sessions, err := r.collectSessionPRCandidates(ctx, &rep)
	if err != nil {
		return rep, err
	}
	items, err := r.collectItemPRCandidates(ctx, &rep)
	if err != nil {
		return rep, err
	}
	seeds := mergeBackfillSeeds(sessions, items)
	for _, group := range [][]prBackfillCandidate{sessions, items} {
		if err := r.importPRCandidates(ctx, group, seeds, hook, &rep); err != nil {
			return rep, err
		}
	}
	logPRBackfill(rep)
	return rep, nil
}

// mergeBackfillSeeds: a PR is an unverified terminal row only when every
// contributing candidate is terminal-seeded (non-terminal wins, fail closed).
func mergeBackfillSeeds(groups ...[]prBackfillCandidate) map[domain.PRRef]bool {
	seeds := make(map[domain.PRRef]bool)
	for _, g := range groups {
		for _, c := range g {
			if prev, ok := seeds[c.ref]; ok {
				seeds[c.ref] = prev && c.terminal
			} else {
				seeds[c.ref] = c.terminal
			}
		}
	}
	return seeds
}

func (r *EntRepository) importPRCandidates(ctx context.Context, group []prBackfillCandidate, seeds map[domain.PRRef]bool, hook prBackfillHook, rep *PRBackfillReport) error {
	imported := 0
	for _, c := range group {
		if imported >= maxBackfillWritesPerBoot {
			rep.Deferred++
			continue
		}
		if hook != nil {
			if err := hook(c.ref); err != nil {
				return err
			}
		}
		res, err := r.UpsertSessionPR(ctx, c.owner, c.ref, PRLink{
			Role: domain.PRRolePrimary, Source: domain.AttachSourceBackfill,
			Initial: PRInitial{State: domain.PRStateUnknown, Terminal: seeds[c.ref], URL: c.url},
		})
		switch {
		case err == nil && res.Tombstoned:
			rep.SkippedTombstoned++
		case err == nil:
			recordImported(rep, c, res)
			imported++
		case isDeterministicPRImportError(err):
			if c.isItem {
				rep.SkippedItemInvalid++
			} else {
				rep.SkippedInvalid++
			}
			rep.skipTitle(c.title)
		default:
			return fmt.Errorf("import %s for %q: %w", c.ref, c.title, err)
		}
	}
	return nil
}

func recordImported(rep *PRBackfillReport, c prBackfillCandidate, res PRLinkResult) {
	if !res.Created {
		return
	}
	if c.isItem {
		rep.CreatedItemLinks++
		return
	}
	rep.CreatedSessionLinks++
	if c.hadLinks {
		rep.ImportedUnrepresented++
	}
}

// isDeterministicPRImportError reports failures that would repeat identically on
// every boot. Anything unrecognized is treated as transient.
func isDeterministicPRImportError(err error) bool {
	return errors.Is(err, ErrZeroPRRef) || errors.Is(err, ErrInvalidPROwner) || errors.Is(err, ErrSessionPRLinkCap) ||
		ent.IsValidationError(err) || ent.IsConstraintError(err)
}

func logPRBackfill(rep PRBackfillReport) {
	log.InfoLog().Printf("[Migration] session_pr backfill: created=%d imported_unrepresented=%d skipped=%d skipped_no_uuid=%d skipped_no_repo=%d skipped_invalid=%d skipped_item_bad_ref=%d skipped_item_invalid=%d skipped_tombstoned=%d deferred=%d titles=%v",
		rep.CreatedSessionLinks+rep.CreatedItemLinks, rep.ImportedUnrepresented,
		rep.SkippedNoRepo+rep.SkippedNoUUID+rep.SkippedInvalid+rep.SkippedItemBadRef+rep.SkippedItemInvalid+rep.SkippedTombstoned,
		rep.SkippedNoUUID, rep.SkippedNoRepo, rep.SkippedInvalid, rep.SkippedItemBadRef, rep.SkippedItemInvalid,
		rep.SkippedTombstoned, rep.Deferred, rep.SkippedTitles)
}

// ---- session candidates ----------------------------------------------------------

type sessionPRRow struct {
	uuid, title string
	ref         domain.PRRef
	terminal    bool
}

func (r *EntRepository) collectSessionPRCandidates(ctx context.Context, rep *PRBackfillReport) ([]prBackfillCandidate, error) {
	rows, err := r.Client.Session.Query().
		Where(entsession.GithubPrNumberGT(0)).
		Select(entsession.FieldUUID, entsession.FieldTitle, entsession.FieldGithubPrNumber, entsession.FieldGithubOwner,
			entsession.FieldGithubRepo, entsession.FieldGithubHost, entsession.FieldGithubPrStatusTerminal,
			entsession.FieldPath, entsession.FieldWorkingDir).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list session pr candidates: %w", err)
	}
	var usable []sessionPRRow
	for _, row := range rows {
		path := row.Path
		if path == "" {
			path = row.WorkingDir
		}
		ref, err := sessionLegacyRef(row.GithubOwner, row.GithubRepo, row.GithubHost, path, row.GithubPrNumber)
		switch {
		case errors.Is(err, errSessionNoRepo):
			rep.SkippedNoRepo++
			rep.skipTitle(row.Title)
		case err != nil:
			rep.SkippedInvalid++
			rep.skipTitle(row.Title)
		case row.UUID == "":
			rep.SkippedNoUUID++ // a later load assigns and persists a uuid; the next boot imports
			rep.skipTitle(row.Title)
		default:
			usable = append(usable, sessionPRRow{uuid: row.UUID, title: row.Title, ref: ref, terminal: row.GithubPrStatusTerminal})
		}
	}
	return r.filterUnrepresentedSessions(ctx, usable, rep)
}

func (r *EntRepository) filterUnrepresentedSessions(ctx context.Context, rows []sessionPRRow, rep *PRBackfillReport) ([]prBackfillCandidate, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	uuids := make([]string, len(rows))
	for i, row := range rows {
		uuids[i] = row.uuid
	}
	links, err := r.linkRefsByOwner(ctx, "session_uuid", uuids)
	if err != nil {
		return nil, err
	}
	tombs, err := r.ListSessionTombstonesByUUIDs(ctx, uuids)
	if err != nil {
		return nil, err
	}
	var out []prBackfillCandidate
	for _, row := range rows {
		switch {
		case containsRef(links[row.uuid], row.ref):
		case containsRef(tombs[row.uuid], row.ref):
			rep.SkippedTombstoned++
		default:
			out = append(out, prBackfillCandidate{
				owner: SessionPROwner(row.uuid), ref: row.ref, title: row.title,
				url: synthesizePRURL(row.ref), terminal: row.terminal, hadLinks: len(links[row.uuid]) > 0,
			})
		}
	}
	return out, nil
}

// linkRefsByOwner returns each owner's linked PRRefs in one joined statement per
// chunk of 500 owners. ownerColumn is a code constant, never caller input.
func (r *EntRepository) linkRefsByOwner(ctx context.Context, ownerColumn string, ids []string) (map[string][]domain.PRRef, error) {
	out := make(map[string][]domain.PRRef)
	for _, chunk := range chunkSlice(ids, itemSessionBatchChunk) {
		query := "SELECT sp." + ownerColumn + ", ps.host, ps.owner, ps.repo, ps.number FROM session_prs sp " +
			"JOIN pull_request_states ps ON ps.id = sp.pull_request_state_id WHERE sp." + ownerColumn +
			" IN (" + strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",") + ")"
		args := make([]any, len(chunk))
		for i, id := range chunk {
			args[i] = id
		}
		rows := &entsql.Rows{}
		if err := r.driver.Query(ctx, query, args, rows); err != nil {
			return nil, fmt.Errorf("list link refs by %s: %w", ownerColumn, err)
		}
		err := scanLinkRefs(rows, out)
		_ = rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func scanLinkRefs(rows *entsql.Rows, out map[string][]domain.PRRef) error {
	for rows.Next() {
		var owner, host, ghOwner, repo string
		var number int
		if err := rows.Scan(&owner, &host, &ghOwner, &repo, &number); err != nil {
			return fmt.Errorf("scan link ref: %w", err)
		}
		ref, err := domain.NewPRRef(host, ghOwner, repo, number)
		if err != nil {
			return fmt.Errorf("stored link ref %s/%s/%s#%d: %w", host, ghOwner, repo, number, err)
		}
		out[owner] = append(out[owner], ref)
	}
	return rows.Err()
}

// ---- item candidates -------------------------------------------------------------

func (r *EntRepository) collectItemPRCandidates(ctx context.Context, rep *PRBackfillReport) ([]prBackfillCandidate, error) {
	items, err := r.Client.BacklogItem.Query().
		Where(backlogitem.PrNumberGT(0)).
		Select(backlogitem.FieldID, backlogitem.FieldTitle, backlogitem.FieldPrURL, backlogitem.FieldPrNumber, backlogitem.FieldStatus).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list item pr candidates: %w", err)
	}
	var out []prBackfillCandidate
	for _, chunk := range chunkSlice(items, itemSessionBatchChunk) {
		got, err := r.unrepresentedItemPRs(ctx, chunk, rep)
		if err != nil {
			return nil, err
		}
		out = append(out, got...)
	}
	return out, nil
}

func (r *EntRepository) unrepresentedItemPRs(ctx context.Context, items []*ent.BacklogItem, rep *PRBackfillReport) ([]prBackfillCandidate, error) {
	ids := make([]string, len(items))
	uuids := make([]uuid.UUID, len(items))
	for i, it := range items {
		ids[i], uuids[i] = it.ID.String(), it.ID
	}
	cov, err := r.itemCoverage(ctx, ids, uuids)
	if err != nil {
		return nil, err
	}
	var out []prBackfillCandidate
	for _, it := range items {
		id := it.ID.String()
		ref, unrepresented, perr := ItemHasUnrepresentedLegacyPR(it.PrURL, it.PrNumber, cov(id))
		switch {
		case perr != nil:
			rep.SkippedItemBadRef++
			rep.skipTitle(it.Title)
		case unrepresented:
			url := strings.TrimSpace(it.PrURL)
			out = append(out, prBackfillCandidate{
				owner: ItemPROwner(id), ref: ref, title: it.Title, url: url,
				terminal: it.Status == "done", isItem: true,
			})
		}
	}
	return out, nil
}

// itemCoverage loads, for one chunk of item ids, the links and tombstones of the
// items and of every session ever attached to them, and returns a per-item view.
func (r *EntRepository) itemCoverage(ctx context.Context, itemIDs []string, itemUUIDs []uuid.UUID) (func(string) ItemPRCoverage, error) {
	sessionsByItem, allSessions, err := r.sessionsOfItems(ctx, itemUUIDs)
	if err != nil {
		return nil, err
	}
	itemLinks, err := r.linkRefsByOwner(ctx, "backlog_item_id", itemIDs)
	if err != nil {
		return nil, err
	}
	sessionLinks, err := r.linkRefsByOwner(ctx, "session_uuid", allSessions)
	if err != nil {
		return nil, err
	}
	itemTombs, err := r.ListItemTombstones(ctx, itemIDs)
	if err != nil {
		return nil, err
	}
	sessionTombs, err := r.ListSessionTombstonesByUUIDs(ctx, allSessions)
	if err != nil {
		return nil, err
	}
	return func(itemID string) ItemPRCoverage {
		c := ItemPRCoverage{
			ItemLinks: itemLinks[itemID], ItemTombstones: itemTombs[itemID],
			SessionLinks: map[string][]domain.PRRef{}, SessionTombstones: map[string][]domain.PRRef{},
		}
		for _, s := range sessionsByItem[itemID] {
			c.SessionLinks[s] = sessionLinks[s]
			c.SessionTombstones[s] = sessionTombs[s]
		}
		return c
	}, nil
}

// sessionsOfItems maps each item to the uuids of every ItemSession it ever had
// (any round and role) and returns the distinct union.
func (r *EntRepository) sessionsOfItems(ctx context.Context, itemIDs []uuid.UUID) (map[string][]string, []string, error) {
	byItem := make(map[string][]string, len(itemIDs))
	seen := map[string]struct{}{}
	var all []string
	for _, chunk := range chunkSlice(itemIDs, itemSessionBatchChunk) {
		rows, err := r.Client.ItemSession.Query().
			Where(itemsession.HasBacklogItemWith(backlogitem.IDIn(chunk...)), itemsession.SessionUUIDNEQ("")).
			WithBacklogItem(func(q *ent.BacklogItemQuery) { q.Select(backlogitem.FieldID) }).
			All(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list item sessions for %d items: %w", len(chunk), err)
		}
		for _, is := range rows {
			if is.Edges.BacklogItem == nil {
				continue
			}
			id := is.Edges.BacklogItem.ID.String()
			byItem[id] = append(byItem[id], is.SessionUUID)
			if _, dup := seen[is.SessionUUID]; !dup {
				seen[is.SessionUUID] = struct{}{}
				all = append(all, is.SessionUUID)
			}
		}
	}
	return byItem, all, nil
}
