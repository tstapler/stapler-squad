package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/predicate"
	"github.com/tstapler/stapler-squad/session/ent/pullrequeststate"
	entsession "github.com/tstapler/stapler-squad/session/ent/session"
	"github.com/tstapler/stapler-squad/session/ent/sessionpr"
	"github.com/tstapler/stapler-squad/session/ent/sessionprtombstone"
)

// Repository for PullRequestState (one row per PR), SessionPR (links) and
// SessionPRTombstone (detaches). Every entry point takes a PRRef built by
// NewPRRef and rejects the zero value with ErrZeroPRRef.
//
// Invariant I1: UpdatePullRequestState (and ResetVerifyCounters for seeded rows)
// are the only writers of state columns after insert. Everyone else inserts a
// state row with EnsureState, which never updates an existing one.

var (
	// ErrInvalidPROwner means an owner did not name exactly one of a session and an item.
	ErrInvalidPROwner = errors.New("session: PR owner must set exactly one of session uuid and backlog item id")
	// ErrSessionPRLinkCap means a session or item already holds maxSessionPRLinks links.
	ErrSessionPRLinkCap = errors.New("session: session already has the maximum number of PR links")
	// ErrNoSessionUUID means the session has no uuid, so no link row can reference it.
	ErrNoSessionUUID = errors.New("session: session has no uuid")
	// ErrSessionNotPersisted means the session row does not exist yet; the first
	// full save writes the PR set instead.
	ErrSessionNotPersisted = errors.New("session: session row not persisted yet")
)

const (
	maxSessionPRLinks = 50
	// prStateIDChunk bounds `id IN (...)` state reads. State ids are integers and
	// one link query can reference many distinct PRs, so this is larger than
	// itemSessionBatchChunk to keep the documented statement budget (links then
	// states) for a few hundred sessions; still far under SQLite's variable limit.
	prStateIDChunk = 5000
	// prDropWarnAfter consecutive dropped observations on one row log a warning.
	prDropWarnAfter = 3
	// prObservationCASRetries bounds the optimistic-concurrency loop in UpdatePullRequestState.
	prObservationCASRetries = 3
)

// PROwner names who a link or tombstone belongs to: exactly one field is set.
type PROwner struct {
	SessionUUID   string
	BacklogItemID string
}

// SessionPROwner returns the owner for a session link.
func SessionPROwner(sessionUUID string) PROwner { return PROwner{SessionUUID: sessionUUID} }

// ItemPROwner returns the owner for a backlog-item-direct link.
func ItemPROwner(itemID string) PROwner { return PROwner{BacklogItemID: itemID} }

func (o PROwner) validate() error {
	if (o.SessionUUID == "") == (o.BacklogItemID == "") {
		return ErrInvalidPROwner
	}
	return nil
}

func (o PROwner) isSession() bool { return o.SessionUUID != "" }

func checkPRIdentity(owner PROwner, ref domain.PRRef) error {
	if ref.IsZero() {
		return ErrZeroPRRef
	}
	return owner.validate()
}

// PRInitial is the state a caller actually observed when inserting a state row;
// the zero value inserts `unknown`, non-terminal (it observed nothing).
type PRInitial struct {
	State      domain.PRState
	Terminal   bool
	URL        string
	Title      string
	HeadBranch string
	BaseBranch string
	IsDraft    bool
}

func (i PRInitial) state() domain.PRState {
	if i.State == "" {
		return domain.PRStateUnknown
	}
	return i.State
}

// PRLink describes the link UpsertSessionPR should create.
type PRLink struct {
	Role    domain.PRRole // requested role; primary is downgraded to followup when the session already has one
	Source  domain.AttachSource
	Initial PRInitial
	// AttachedAt, when set, is stored as the link's attached_at instead of now.
	AttachedAt time.Time
}

// PRLinkResult reports what UpsertSessionPR did.
type PRLinkResult struct {
	StateID    int
	Created    bool // a new link row was inserted
	Tombstoned bool // the owner detached this PR earlier; nothing was written
	Role       domain.PRRole
}

func clientEnsureState(ctx context.Context, c *ent.PullRequestStateClient, ref domain.PRRef, initial PRInitial) (int, error) {
	create := c.Create().
		SetHost(ref.Host()).SetOwner(ref.Owner()).SetRepo(ref.Repo()).SetNumber(ref.Number()).
		SetState(string(initial.state())).SetTerminal(initial.Terminal).
		SetURL(initial.URL).SetTitle(initial.Title).SetHeadBranch(initial.HeadBranch).SetBaseBranch(initial.BaseBranch).
		SetIsDraft(initial.IsDraft)
	err := create.OnConflictColumns(pullrequeststate.FieldHost, pullrequeststate.FieldOwner, pullrequeststate.FieldRepo, pullrequeststate.FieldNumber).
		DoNothing().Exec(ctx)
	// SQLite surfaces DO NOTHING on an existing row as sql.ErrNoRows from RETURNING.
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("ensure pull request state %s: %w", ref, err)
	}
	return clientStateID(ctx, c, ref)
}

func clientStateID(ctx context.Context, c *ent.PullRequestStateClient, ref domain.PRRef) (int, error) {
	id, err := c.Query().Where(refPredicate(ref)).OnlyID(ctx)
	if err != nil {
		return 0, fmt.Errorf("look up pull request state %s: %w", ref, err)
	}
	return id, nil
}

func refPredicate(ref domain.PRRef) predicate.PullRequestState {
	return pullrequeststate.And(
		pullrequeststate.Host(ref.Host()), pullrequeststate.Owner(ref.Owner()),
		pullrequeststate.Repo(ref.Repo()), pullrequeststate.Number(ref.Number()),
	)
}

// EnsureState inserts the state row for ref if absent and returns its id. It
// never modifies an existing row.
func (r *EntRepository) EnsureState(ctx context.Context, ref domain.PRRef, initial PRInitial) (int, error) {
	if ref.IsZero() {
		return 0, ErrZeroPRRef
	}
	return clientEnsureState(ctx, r.Client.PullRequestState, ref, initial)
}

func tombstonePredicate(owner PROwner, ref domain.PRRef) []predicate.SessionPRTombstone {
	return []predicate.SessionPRTombstone{
		sessionprtombstone.SessionUUID(owner.SessionUUID), sessionprtombstone.BacklogItemID(owner.BacklogItemID),
		sessionprtombstone.Host(ref.Host()), sessionprtombstone.Owner(ref.Owner()),
		sessionprtombstone.Repo(ref.Repo()), sessionprtombstone.Number(ref.Number()),
	}
}

func linkOwnerPredicates(owner PROwner) []predicate.SessionPR {
	return []predicate.SessionPR{sessionpr.SessionUUID(owner.SessionUUID), sessionpr.BacklogItemID(owner.BacklogItemID)}
}

// IsTombstoned reports whether owner detached ref earlier.
func (r *EntRepository) IsTombstoned(ctx context.Context, owner PROwner, ref domain.PRRef) (bool, error) {
	if err := checkPRIdentity(owner, ref); err != nil {
		return false, err
	}
	return r.Client.SessionPRTombstone.Query().Where(tombstonePredicate(owner, ref)...).Exist(ctx)
}

// UpsertSessionPR ensures the state row and the owner's link in one transaction.
// A tombstoned (owner, ref) writes nothing and reports Tombstoned; a user attach
// calls ClearTombstone first. An existing link is left untouched (attached_at,
// role and pin survive a repeat call).
func (r *EntRepository) UpsertSessionPR(ctx context.Context, owner PROwner, ref domain.PRRef, link PRLink) (PRLinkResult, error) {
	if err := checkPRIdentity(owner, ref); err != nil {
		return PRLinkResult{}, err
	}
	var res PRLinkResult
	err := r.withPRTx(ctx, "upsert session pr", func(tx *ent.Tx) error {
		var err error
		res, err = upsertSessionPRTx(ctx, tx, owner, ref, link)
		return err
	})
	return res, err
}

func upsertSessionPRTx(ctx context.Context, tx *ent.Tx, owner PROwner, ref domain.PRRef, link PRLink) (PRLinkResult, error) {
	tombstoned, err := tx.SessionPRTombstone.Query().Where(tombstonePredicate(owner, ref)...).Exist(ctx)
	if err != nil {
		return PRLinkResult{}, fmt.Errorf("check tombstone: %w", err)
	}
	if tombstoned {
		return PRLinkResult{Tombstoned: true}, nil
	}
	stateID, err := clientEnsureState(ctx, tx.PullRequestState, ref, link.Initial)
	if err != nil {
		return PRLinkResult{}, err
	}
	existing, err := tx.SessionPR.Query().
		Where(append(linkOwnerPredicates(owner), sessionpr.PullRequestStateID(stateID))...).Only(ctx)
	switch {
	case err == nil:
		return PRLinkResult{StateID: stateID, Role: domain.PRRole(existing.Role)}, nil
	case !ent.IsNotFound(err):
		return PRLinkResult{}, fmt.Errorf("look up existing link: %w", err)
	}
	role, err := resolveNewLinkRole(ctx, tx, owner, link.Role)
	if err != nil {
		return PRLinkResult{}, err
	}
	create := tx.SessionPR.Create().
		SetPullRequestStateID(stateID).SetSessionUUID(owner.SessionUUID).SetBacklogItemID(owner.BacklogItemID).
		SetRole(string(role)).SetAttachSource(string(link.Source))
	if !link.AttachedAt.IsZero() {
		create.SetAttachedAt(link.AttachedAt)
	}
	if err := create.Exec(ctx); err != nil {
		return PRLinkResult{}, fmt.Errorf("create session pr link: %w", err)
	}
	return PRLinkResult{StateID: stateID, Created: true, Role: role}, nil
}

// resolveNewLinkRole enforces the link cap for sessions and items, and the
// one-primary rule for sessions.
func resolveNewLinkRole(ctx context.Context, tx *ent.Tx, owner PROwner, want domain.PRRole) (domain.PRRole, error) {
	if want == "" {
		want = domain.PRRolePrimary
	}
	count, err := tx.SessionPR.Query().Where(linkOwnerPredicates(owner)...).Count(ctx)
	if err != nil {
		return "", fmt.Errorf("count owner links: %w", err)
	}
	if count >= maxSessionPRLinks {
		return "", ErrSessionPRLinkCap
	}
	if !owner.isSession() {
		return want, nil
	}
	if want != domain.PRRolePrimary {
		return want, nil
	}
	hasPrimary, err := tx.SessionPR.Query().
		Where(sessionpr.SessionUUID(owner.SessionUUID), sessionpr.Role(string(domain.PRRolePrimary))).Exist(ctx)
	if err != nil {
		return "", fmt.Errorf("check primary link: %w", err)
	}
	if hasPrimary {
		return domain.PRRoleFollowup, nil
	}
	return domain.PRRolePrimary, nil
}

// withPRTx runs fn in a transaction, rolling back on error.
func (r *EntRepository) withPRTx(ctx context.Context, op string, fn func(*ent.Tx) error) error {
	tx, err := r.Client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("%s: begin transaction: %w", op, err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s: commit: %w", op, err)
	}
	return nil
}

// AttachSessionPR ensures the state row, the session's link and the legacy mirror
// columns in ONE transaction, keyed by session uuid. It returns ErrNoSessionUUID
// for an empty uuid and ErrSessionNotPersisted when the session row does not exist
// yet (the first full save then writes the set). A tombstoned PR writes nothing
// and reports Tombstoned. Failure at any step rolls everything back.
func (r *EntRepository) AttachSessionPR(ctx context.Context, sessionUUID string, ref domain.PRRef, link PRLink, mirror PRMirror) (PRLinkResult, error) {
	if sessionUUID == "" {
		return PRLinkResult{}, ErrNoSessionUUID
	}
	if ref.IsZero() {
		return PRLinkResult{}, ErrZeroPRRef
	}
	var res PRLinkResult
	err := r.withPRTx(ctx, "attach session pr", func(tx *ent.Tx) error {
		rowID, err := tx.Session.Query().Where(entsession.UUID(sessionUUID)).OnlyID(ctx)
		if ent.IsNotFound(err) {
			return ErrSessionNotPersisted
		} else if err != nil {
			return fmt.Errorf("look up session: %w", err)
		}
		if res, err = upsertSessionPRTx(ctx, tx, SessionPROwner(sessionUUID), ref, link); err != nil || res.Tombstoned {
			return err
		}
		if r.afterPRLinkHook != nil {
			if err := r.afterPRLinkHook(); err != nil {
				return err
			}
		}
		return setMirrorTx(ctx, tx, rowID, mirror)
	})
	return res, err
}

// PersistPRMirror writes the legacy scalar PR columns of the session with this
// uuid, including clearing them (number 0, empty URL, terminal false), which the
// full-save set-only path cannot. It returns ErrSessionNotPersisted when no row matches.
func (r *EntRepository) PersistPRMirror(ctx context.Context, sessionUUID string, mirror PRMirror) error {
	if sessionUUID == "" {
		return ErrNoSessionUUID
	}
	return r.withPRTx(ctx, "persist pr mirror", func(tx *ent.Tx) error {
		rowID, err := tx.Session.Query().Where(entsession.UUID(sessionUUID)).OnlyID(ctx)
		if ent.IsNotFound(err) {
			return ErrSessionNotPersisted
		} else if err != nil {
			return fmt.Errorf("look up session: %w", err)
		}
		return setMirrorTx(ctx, tx, rowID, mirror)
	})
}

func setMirrorTx(ctx context.Context, tx *ent.Tx, rowID int, m PRMirror) error {
	err := tx.Session.UpdateOneID(rowID).
		SetGithubPrURL(m.URL).SetGithubPrNumber(m.Number).SetGithubPrStatusTerminal(m.Terminal).Exec(ctx)
	if err != nil {
		return fmt.Errorf("update pr mirror columns: %w", err)
	}
	return nil
}

// upsertSessionPRSetTx idempotently writes the links (and, insert-if-absent, their
// state rows) carried by a full save. It never updates a state column (invariant I1),
// so a stale snapshot cannot overwrite a fresher poll, and a tombstoned PR is skipped.
func upsertSessionPRSetTx(ctx context.Context, tx *ent.Tx, data domain.InstanceData) error {
	if data.UUID == "" {
		return nil
	}
	owner := SessionPROwner(data.UUID)
	for _, v := range data.PRs {
		link := PRLink{Role: v.Role, Source: v.AttachSource, Initial: prInitialOf(v), AttachedAt: v.AttachedAt}
		if _, err := upsertSessionPRTx(ctx, tx, owner, v.Ref, link); err != nil {
			if isDeterministicPRImportError(err) {
				// A bad link must not roll back the rest of the session save.
				log.Warn("save pr link skipped", "pr", v.Ref.String(), "err", err)
				continue
			}
			return fmt.Errorf("save pr link %s: %w", v.Ref, err)
		}
	}
	return nil
}

// ClearTombstone removes one owner's tombstone for ref: the repository half of a
// user attach. It is a no-op when none exists.
func (r *EntRepository) ClearTombstone(ctx context.Context, owner PROwner, ref domain.PRRef) error {
	if err := checkPRIdentity(owner, ref); err != nil {
		return err
	}
	_, err := r.Client.SessionPRTombstone.Delete().Where(tombstonePredicate(owner, ref)...).Exec(ctx)
	return err
}

// DeleteSessionPR detaches ref from owner: it deletes exactly that owner's link
// and writes that owner's tombstone in one transaction. It reports whether a link
// existed; the tombstone is written either way so a later automatic attach is blocked.
func (r *EntRepository) DeleteSessionPR(ctx context.Context, owner PROwner, ref domain.PRRef) (bool, error) {
	if err := checkPRIdentity(owner, ref); err != nil {
		return false, err
	}
	deleted := false
	err := r.withPRTx(ctx, "delete session pr", func(tx *ent.Tx) error {
		stateID, err := tx.PullRequestState.Query().Where(refPredicate(ref)).OnlyID(ctx)
		switch {
		case err == nil:
			n, derr := tx.SessionPR.Delete().
				Where(append(linkOwnerPredicates(owner), sessionpr.PullRequestStateID(stateID))...).Exec(ctx)
			if derr != nil {
				return fmt.Errorf("delete link: %w", derr)
			}
			deleted = n > 0
		case !ent.IsNotFound(err):
			return fmt.Errorf("look up state: %w", err)
		}
		terr := tx.SessionPRTombstone.Create().
			SetSessionUUID(owner.SessionUUID).SetBacklogItemID(owner.BacklogItemID).
			SetHost(ref.Host()).SetOwner(ref.Owner()).SetRepo(ref.Repo()).SetNumber(ref.Number()).
			OnConflictColumns(sessionprtombstone.FieldSessionUUID, sessionprtombstone.FieldBacklogItemID,
				sessionprtombstone.FieldHost, sessionprtombstone.FieldOwner, sessionprtombstone.FieldRepo, sessionprtombstone.FieldNumber).
			DoNothing().Exec(ctx)
		if terr != nil && !errors.Is(terr, sql.ErrNoRows) {
			return fmt.Errorf("write tombstone: %w", terr)
		}
		return nil
	})
	return deleted, err
}

// SetPinned pins or unpins ref on a session in one transaction; pinning clears
// every other pin of that session. Only sessions can pin.
func (r *EntRepository) SetPinned(ctx context.Context, owner PROwner, ref domain.PRRef, pinned bool) error {
	if err := checkPRIdentity(owner, ref); err != nil {
		return err
	}
	if !owner.isSession() {
		return ErrInvalidPROwner
	}
	return r.withPRTx(ctx, "set pinned", func(tx *ent.Tx) error {
		stateID, err := tx.PullRequestState.Query().Where(refPredicate(ref)).OnlyID(ctx)
		if err != nil {
			return fmt.Errorf("look up state %s: %w", ref, err)
		}
		linkID, err := tx.SessionPR.Query().
			Where(sessionpr.SessionUUID(owner.SessionUUID), sessionpr.PullRequestStateID(stateID)).OnlyID(ctx)
		if err != nil {
			return fmt.Errorf("look up link: %w", err)
		}
		if pinned {
			if _, err := tx.SessionPR.Update().
				Where(sessionpr.SessionUUID(owner.SessionUUID), sessionpr.IDNEQ(linkID), sessionpr.Pinned(true)).
				SetPinned(false).Save(ctx); err != nil {
				return fmt.Errorf("clear other pins: %w", err)
			}
		}
		return tx.SessionPR.UpdateOneID(linkID).SetPinned(pinned).Exec(ctx)
	})
}

// ---- reads -------------------------------------------------------------------

func chunkSlice[T any](xs []T, size int) [][]T {
	var out [][]T
	for len(xs) > size {
		out = append(out, xs[:size])
		xs = xs[size:]
	}
	if len(xs) > 0 {
		out = append(out, xs)
	}
	return out
}

// ListBySessionUUIDs returns each session's links joined to their states.
// Statements: one per 500 uuids for links, plus one per prStateIDChunk distinct
// states. Sessions without links are absent from the map.
func (r *EntRepository) ListBySessionUUIDs(ctx context.Context, uuids []string) (map[string][]domain.PRView, error) {
	out := make(map[string][]domain.PRView, len(uuids))
	var links []*ent.SessionPR
	for _, chunk := range chunkSlice(uuids, itemSessionBatchChunk) {
		got, err := r.Client.SessionPR.Query().Where(sessionpr.SessionUUIDIn(chunk...)).All(ctx)
		if err != nil {
			return nil, fmt.Errorf("list session prs: %w", err)
		}
		links = append(links, got...)
	}
	views, err := r.joinLinkStates(ctx, links)
	if err != nil {
		return nil, err
	}
	for _, v := range views {
		out[v.SessionUUID] = append(out[v.SessionUUID], v)
	}
	return out, nil
}

// SessionUUIDsByPR returns the sorted UUIDs of the sessions linked to ref
// (item-direct links excluded). Two statements: the state row, then its links.
// Rows are not checked against live sessions; the caller filters orphans.
func (r *EntRepository) SessionUUIDsByPR(ctx context.Context, ref domain.PRRef) ([]string, error) {
	if ref.IsZero() {
		return nil, ErrZeroPRRef
	}
	stateID, err := r.Client.PullRequestState.Query().Where(refPredicate(ref)).OnlyID(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("look up pull request state %s: %w", ref, err)
	}
	uuids, err := r.Client.SessionPR.Query().
		Where(sessionpr.PullRequestStateID(stateID), sessionpr.SessionUUIDNEQ("")).
		Order(ent.Asc(sessionpr.FieldSessionUUID)).
		Select(sessionpr.FieldSessionUUID).Strings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list sessions linked to %s: %w", ref, err)
	}
	return uuids, nil
}

// ListByItem returns the item-direct links of one backlog item joined to states.
func (r *EntRepository) ListByItem(ctx context.Context, itemID string) ([]domain.PRView, error) {
	if itemID == "" {
		return nil, ErrInvalidPROwner
	}
	links, err := r.Client.SessionPR.Query().Where(sessionpr.BacklogItemID(itemID)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list item prs: %w", err)
	}
	return r.joinLinkStates(ctx, links)
}

func (r *EntRepository) joinLinkStates(ctx context.Context, links []*ent.SessionPR) ([]domain.PRView, error) {
	if len(links) == 0 {
		return nil, nil
	}
	seen := make(map[int]struct{}, len(links))
	ids := make([]int, 0, len(links))
	for _, l := range links {
		if _, ok := seen[l.PullRequestStateID]; !ok {
			seen[l.PullRequestStateID] = struct{}{}
			ids = append(ids, l.PullRequestStateID)
		}
	}
	states := make(map[int]*ent.PullRequestState, len(ids))
	for _, chunk := range chunkSlice(ids, prStateIDChunk) {
		got, err := r.Client.PullRequestState.Query().Where(pullrequeststate.IDIn(chunk...)).All(ctx)
		if err != nil {
			return nil, fmt.Errorf("load pull request states: %w", err)
		}
		for _, s := range got {
			states[s.ID] = s
		}
	}
	views := make([]domain.PRView, 0, len(links))
	for _, l := range links {
		st, ok := states[l.PullRequestStateID]
		if !ok {
			continue // FK guarantees presence; skip rather than invent a ref
		}
		v, err := r.linkStateToView(st, l)
		if err != nil {
			return nil, err
		}
		views = append(views, v)
	}
	return views, nil
}

// ListPollableStates is the poller's work list: states with at least one link
// that are non-terminal, plus unverified-terminal states whose verification is
// not exhausted. Non-terminal states come first, then unverified terminal ones,
// each newest link (attached_at) first. Returned views carry state columns only;
// AttachedAt is the newest link's, and owner fields are empty.
func (r *EntRepository) ListPollableStates(ctx context.Context) ([]domain.PRView, error) {
	states, err := r.Client.PullRequestState.Query().
		Where(pullrequeststate.HasLinks(), pullrequeststate.Or(
			pullrequeststate.Terminal(false),
			pullrequeststate.And(pullrequeststate.State(string(domain.PRStateUnknown)), pullrequeststate.Terminal(true)),
		)).
		WithLinks().
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pollable states: %w", err)
	}
	out := make([]domain.PRView, 0, len(states))
	for _, st := range states {
		v, err := r.linkStateToView(st, nil)
		if err != nil {
			return nil, err
		}
		for _, l := range st.Edges.Links {
			if l.AttachedAt.After(v.AttachedAt) {
				v.AttachedAt = l.AttachedAt
			}
		}
		if v.UnverifiedTerminal() && v.VerifyExhausted() {
			continue
		}
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if ti, tj := out[i].UnverifiedTerminal(), out[j].UnverifiedTerminal(); ti != tj {
			return !ti
		}
		return out[i].AttachedAt.After(out[j].AttachedAt)
	})
	return out, nil
}

// GetPullRequestState returns one PR's state columns (no owner fields).
func (r *EntRepository) GetPullRequestState(ctx context.Context, ref domain.PRRef) (domain.PRView, error) {
	if ref.IsZero() {
		return domain.PRView{}, ErrZeroPRRef
	}
	st, err := r.Client.PullRequestState.Query().Where(refPredicate(ref)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return domain.PRView{}, fmt.Errorf("%w: pull request state %s", domain.ErrNotFound, ref)
		}
		return domain.PRView{}, fmt.Errorf("load pull request state %s: %w", ref, err)
	}
	return r.linkStateToView(st, nil)
}

// linkStateToView converts a state row (and optionally one of its links) to a PRView.
func (r *EntRepository) linkStateToView(st *ent.PullRequestState, link *ent.SessionPR) (domain.PRView, error) {
	ref, err := domain.NewPRRef(st.Host, st.Owner, st.Repo, st.Number)
	if err != nil {
		return domain.PRView{}, fmt.Errorf("stored pull request state %d: %w", st.ID, err)
	}
	state, err := domain.ParsePRState(st.State)
	if err != nil {
		return domain.PRView{}, fmt.Errorf("stored pull request state %d: %w", st.ID, err)
	}
	v := domain.PRView{
		StateID: st.ID, Ref: ref, State: state, StoredTerminal: st.Terminal, URL: st.URL, Title: st.Title,
		HeadBranch: st.HeadBranch, BaseBranch: st.BaseBranch, IsDraft: st.IsDraft,
		CheckConclusion: st.CheckConclusion, ApprovedCount: st.ApprovedCount, ChangesRequestedCount: st.ChangesReqCount,
		OpenedAt: derefTime(st.OpenedAt), MergedAt: derefTime(st.MergedAt), ClosedAt: derefTime(st.ClosedAt),
		LastCheckedAt: derefTime(st.LastCheckedAt), LastPolledAt: derefTime(st.LastPolledAt),
		PollFailCount: st.PollFailCount, FirstFailAt: derefTime(st.FirstFailAt), PollError: st.PollError,
		RateLimitResetAt: derefTime(st.RateLimitResetAt),
	}
	if v.UnverifiedTerminal() && !r.prVerifyCountersTrusted.Load() {
		// Counters left by an earlier process are not trusted until this process's
		// maintenance pass has succeeded, so such a row reads as not exhausted.
		v.PollFailCount, v.FirstFailAt = 0, time.Time{}
	}
	if link != nil {
		role, rerr := domain.ParsePRRole(link.Role)
		if rerr != nil {
			log.Warn("stored session pr role unparseable", "link", link.ID, "role", link.Role)
		}
		src, _ := domain.ParseAttachSource(link.AttachSource)
		v.SessionUUID, v.BacklogItemID = link.SessionUUID, link.BacklogItemID
		v.Role, v.Pinned, v.AttachSource, v.AttachedAt = role, link.Pinned, src, link.AttachedAt
	}
	return v, nil
}

func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// MarkPRMaintenanceSucceeded records that this process's startup maintenance
// pass succeeded, so stored verification counters are trusted from now on.
func (r *EntRepository) MarkPRMaintenanceSucceeded() { r.prVerifyCountersTrusted.Store(true) }

// ListSessionTombstonesByUUIDs returns each session's tombstoned PRs (chunked by 500).
func (r *EntRepository) ListSessionTombstonesByUUIDs(ctx context.Context, uuids []string) (map[string][]domain.PRRef, error) {
	out := make(map[string][]domain.PRRef)
	for _, chunk := range chunkSlice(uuids, itemSessionBatchChunk) {
		rows, err := r.Client.SessionPRTombstone.Query().Where(sessionprtombstone.SessionUUIDIn(chunk...)).All(ctx)
		if err != nil {
			return nil, fmt.Errorf("list session tombstones: %w", err)
		}
		if err := collectTombstones(out, rows, func(t *ent.SessionPRTombstone) string { return t.SessionUUID }); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// ListItemTombstones returns each item's tombstoned PRs (chunked by 500).
func (r *EntRepository) ListItemTombstones(ctx context.Context, itemIDs []string) (map[string][]domain.PRRef, error) {
	out := make(map[string][]domain.PRRef)
	for _, chunk := range chunkSlice(itemIDs, itemSessionBatchChunk) {
		rows, err := r.Client.SessionPRTombstone.Query().Where(sessionprtombstone.BacklogItemIDIn(chunk...)).All(ctx)
		if err != nil {
			return nil, fmt.Errorf("list item tombstones: %w", err)
		}
		if err := collectTombstones(out, rows, func(t *ent.SessionPRTombstone) string { return t.BacklogItemID }); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func collectTombstones(out map[string][]domain.PRRef, rows []*ent.SessionPRTombstone, key func(*ent.SessionPRTombstone) string) error {
	for _, t := range rows {
		ref, err := domain.NewPRRef(t.Host, t.Owner, t.Repo, t.Number)
		if err != nil {
			return fmt.Errorf("stored tombstone %d: %w", t.ID, err)
		}
		out[key(t)] = append(out[key(t)], ref)
	}
	return nil
}

// ---- the single state writer ---------------------------------------------------

// PRObservationSource says where an observation came from.
type PRObservationSource string

const (
	PRSourceTick              PRObservationSource = "tick"
	PRSourceReconcilerRefresh PRObservationSource = "reconciler_refresh"
	PRSourceWebhook           PRObservationSource = "webhook"
	PRSourceWebhookReopened   PRObservationSource = "webhook_reopened"
)

// PRObservationKind is what an observation reports.
type PRObservationKind string

const (
	PRObservationFetch          PRObservationKind = "fetch"           // a successful fetch of the PR
	PRObservationCountedFailure PRObservationKind = "counted_failure" // a failure that counts toward exhaustion
	PRObservationErrorNote      PRObservationKind = "error_note"      // a non-counted failure; writes poll_error only
)

// PRObservation is one observation of a PR. ObservedAt is the injected clock at
// fetch START; Now is the injected clock at apply time (zero means time.Now()).
type PRObservation struct {
	Source     PRObservationSource
	Kind       PRObservationKind
	ObservedAt time.Time
	Now        time.Time

	// Fetch payload (Kind == PRObservationFetch). State must be open, closed or merged.
	State                 domain.PRState
	IsDraft               bool
	CheckConclusion       string
	ApprovedCount         int
	ChangesRequestedCount int
	URL, HeadBranch       string
	Title                 string
	BaseBranch            string
	OpenedAt              time.Time
	MergedAt, ClosedAt    time.Time
	RateLimitResetAt      time.Time

	// PollError is the poll_error enum for the two failure kinds.
	PollError string
}

// PRDropReason says why an observation was not applied.
type PRDropReason string

const (
	PRDropStale  PRDropReason = "stale"
	PRDropSticky PRDropReason = "sticky"
	PRDropSource PRDropReason = "source"
)

// PRObservationResult reports whether UpdatePullRequestState applied the observation.
type PRObservationResult struct {
	Applied bool
	Dropped PRDropReason
}

// PRObservationDrops counts dropped observations by reason.
type PRObservationDrops struct{ Stale, Sticky, Source int64 }

type prDropTracker struct {
	mu          sync.Mutex
	counts      PRObservationDrops
	consecutive map[int]int
}

func (t *prDropTracker) record(stateID int, ref domain.PRRef, reason PRDropReason) {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch reason {
	case PRDropStale:
		t.counts.Stale++
	case PRDropSticky:
		t.counts.Sticky++
	case PRDropSource:
		t.counts.Source++
	}
	if t.consecutive == nil {
		t.consecutive = make(map[int]int)
	}
	t.consecutive[stateID]++
	if n := t.consecutive[stateID]; n == prDropWarnAfter {
		log.Warn("pull request state observations repeatedly dropped", "pr", ref.String(), "reason", string(reason), "consecutive", n)
	} else {
		log.Debug("pull request state observation dropped", "pr", ref.String(), "reason", string(reason))
	}
}

func (t *prDropTracker) applied(stateID int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.consecutive, stateID)
}

// PRObservationDropCounts returns the dropped-observation counters.
func (r *EntRepository) PRObservationDropCounts() PRObservationDrops {
	r.prDrops.mu.Lock()
	defer r.prDrops.mu.Unlock()
	return r.prDrops.counts
}

// UpdatePullRequestState applies one observation to the state row of ref. It is
// the ONLY writer of state columns after insert (invariant I1), monotonic in
// time and sticky on terminal states except for the edges of the transition table:
//
//	open -> open|merged|closed             any source
//	closed -> merged                       any source, later observed_at
//	closed -> open                         only webhook_reopened, strictly later observed_at
//	merged -> anything                     rejected
//	unknown (either terminal bit) -> real  any source
//	open -> unknown                        only via the 5-counted-failure rule
func (r *EntRepository) UpdatePullRequestState(ctx context.Context, ref domain.PRRef, obs PRObservation) (PRObservationResult, error) {
	if ref.IsZero() {
		return PRObservationResult{}, ErrZeroPRRef
	}
	if err := obs.validate(); err != nil {
		return PRObservationResult{}, err
	}
	if obs.Now.IsZero() {
		obs.Now = time.Now()
	}
	for attempt := 0; attempt < prObservationCASRetries; attempt++ {
		row, err := r.Client.PullRequestState.Query().Where(refPredicate(ref)).Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				return PRObservationResult{}, fmt.Errorf("%w: pull request state %s", domain.ErrNotFound, ref)
			}
			return PRObservationResult{}, fmt.Errorf("load pull request state %s: %w", ref, err)
		}
		plan, reason := planObservation(row, obs)
		if reason != "" {
			r.prDrops.record(row.ID, ref, reason)
			return PRObservationResult{Dropped: reason}, nil
		}
		won, err := r.applyObservationPlan(ctx, row, plan)
		if err != nil {
			return PRObservationResult{}, err
		}
		if won {
			r.prDrops.applied(row.ID)
			return PRObservationResult{Applied: true}, nil
		}
	}
	return PRObservationResult{}, fmt.Errorf("update pull request state %s: lost the compare-and-set %d times", ref, prObservationCASRetries)
}

func (o PRObservation) validate() error {
	if o.ObservedAt.IsZero() {
		return errors.New("pr observation: ObservedAt is required")
	}
	switch o.Kind {
	case PRObservationFetch:
		if st, err := domain.ParsePRState(string(o.State)); err != nil || st == domain.PRStateUnknown {
			return fmt.Errorf("pr observation: a fetch must report open, closed or merged, got %q", o.State)
		}
	case PRObservationCountedFailure, PRObservationErrorNote:
		if _, err := validPollError(o.PollError); err != nil {
			return err
		}
	default:
		return fmt.Errorf("pr observation: unknown kind %q", o.Kind)
	}
	switch o.Source {
	case PRSourceTick, PRSourceReconcilerRefresh, PRSourceWebhook, PRSourceWebhookReopened:
	default:
		return fmt.Errorf("pr observation: unknown source %q", o.Source)
	}
	return nil
}

func validPollError(s string) (string, error) {
	switch s {
	case "", "none", "auth", "rate_limited", "not_found", "repo_inaccessible", "network":
		if s == "none" {
			return "", nil
		}
		return s, nil
	}
	return "", fmt.Errorf("pr observation: unknown poll error %q", s)
}

// observationPlan is the full set of column writes for one applied observation.
type observationPlan struct {
	obs PRObservation
	// The row values the plan was computed from, used as the compare-and-set guard.
	oldState    string
	oldTerminal bool
	oldPolled   *time.Time
	// Computed outcome.
	newState     domain.PRState
	newTerminal  bool
	failCount    int
	firstFailAt  *time.Time
	touchPolled  bool
	clearCounter bool
}

// planObservation decides whether obs applies to row and, if so, what to write.
func planObservation(row *ent.PullRequestState, obs PRObservation) (observationPlan, PRDropReason) {
	p := observationPlan{obs: obs, oldState: row.State, oldTerminal: row.Terminal, oldPolled: row.LastPolledAt}
	if row.LastPolledAt != nil && obs.ObservedAt.Before(*row.LastPolledAt) && !row.LastPolledAt.After(obs.Now) {
		return p, PRDropStale
	}
	cur := domain.PRState(row.State)
	if cur == domain.PRStateMerged || (cur == domain.PRStateClosed && obs.Kind != PRObservationFetch) {
		return p, PRDropSticky
	}
	switch obs.Kind {
	case PRObservationFetch:
		return planFetch(p, row, cur)
	case PRObservationCountedFailure:
		return planCountedFailure(p, row, cur)
	default: // error note: poll_error only
		p.newState, p.newTerminal = cur, row.Terminal
		return p, ""
	}
}

func planFetch(p observationPlan, row *ent.PullRequestState, cur domain.PRState) (observationPlan, PRDropReason) {
	obs := p.obs
	if cur == domain.PRStateClosed && obs.State == domain.PRStateOpen {
		reopen := obs.Source == PRSourceWebhookReopened && (row.LastPolledAt == nil || obs.ObservedAt.After(*row.LastPolledAt))
		if !reopen {
			if obs.Source == PRSourceReconcilerRefresh || obs.Source == PRSourceWebhook {
				return p, PRDropSource
			}
			return p, PRDropSticky
		}
	}
	p.newState, p.newTerminal, p.touchPolled, p.clearCounter = obs.State, obs.State.Terminal(), true, true
	return p, ""
}

func planCountedFailure(p observationPlan, row *ent.PullRequestState, cur domain.PRState) (observationPlan, PRDropReason) {
	p.failCount = row.PollFailCount + 1
	p.firstFailAt = row.FirstFailAt
	if row.PollFailCount == 0 || row.FirstFailAt == nil {
		t := p.obs.ObservedAt
		p.firstFailAt = &t
	}
	p.touchPolled = true
	p.newState, p.newTerminal = cur, row.Terminal
	exhausted := p.failCount >= domain.VerifyExhaustedFailures && p.obs.ObservedAt.Sub(*p.firstFailAt) >= domain.VerifyExhaustedSpan
	if cur == domain.PRStateOpen && exhausted {
		p.newState, p.newTerminal = domain.PRStateUnknown, false
	}
	return p, ""
}

// applyObservationPlan writes the plan with a compare-and-set on the columns it
// was computed from; it reports false when another writer changed the row first.
func (r *EntRepository) applyObservationPlan(ctx context.Context, row *ent.PullRequestState, p observationPlan) (bool, error) {
	obs := p.obs
	upd := r.Client.PullRequestState.Update().Where(
		pullrequeststate.ID(row.ID), pullrequeststate.State(p.oldState), pullrequeststate.Terminal(p.oldTerminal),
		lastPolledMatches(p.oldPolled),
	)
	pollErr, _ := validPollError(obs.PollError)
	upd = upd.SetState(string(p.newState)).SetTerminal(p.newTerminal)
	switch obs.Kind {
	case PRObservationFetch:
		upd = applyFetchColumns(upd, obs).SetPollError("").SetPollFailCount(0).ClearFirstFailAt().
			SetLastPolledAt(obs.ObservedAt).SetLastCheckedAt(obs.Now)
	case PRObservationCountedFailure:
		upd = upd.SetPollError(pollErr).SetPollFailCount(p.failCount).SetFirstFailAt(*p.firstFailAt).
			SetLastPolledAt(obs.ObservedAt).SetLastCheckedAt(obs.Now)
		upd = setRateLimitResetIfAny(upd, obs)
	default:
		upd = setRateLimitResetIfAny(upd.SetPollError(pollErr), obs)
	}
	n, err := upd.Save(ctx)
	if err != nil {
		return false, fmt.Errorf("update pull request state %d: %w", row.ID, err)
	}
	return n == 1, nil
}

// setRateLimitResetIfAny persists a rate-limit reset on a failure observation;
// unlike a fetch it never clears one, so a failure without a reset keeps the last.
func setRateLimitResetIfAny(upd *ent.PullRequestStateUpdate, obs PRObservation) *ent.PullRequestStateUpdate {
	if obs.RateLimitResetAt.IsZero() {
		return upd
	}
	return upd.SetRateLimitResetAt(obs.RateLimitResetAt)
}

func applyFetchColumns(upd *ent.PullRequestStateUpdate, obs PRObservation) *ent.PullRequestStateUpdate {
	upd = upd.SetIsDraft(obs.IsDraft).SetCheckConclusion(obs.CheckConclusion).
		SetApprovedCount(obs.ApprovedCount).SetChangesReqCount(obs.ChangesRequestedCount)
	if obs.URL != "" {
		upd = upd.SetURL(obs.URL)
	}
	if obs.Title != "" {
		upd = upd.SetTitle(obs.Title)
	}
	if obs.HeadBranch != "" {
		upd = upd.SetHeadBranch(obs.HeadBranch)
	}
	if obs.BaseBranch != "" {
		upd = upd.SetBaseBranch(obs.BaseBranch)
	}
	if !obs.OpenedAt.IsZero() {
		upd = upd.SetOpenedAt(obs.OpenedAt)
	}
	if !obs.MergedAt.IsZero() {
		upd = upd.SetMergedAt(obs.MergedAt)
	}
	if !obs.ClosedAt.IsZero() {
		upd = upd.SetClosedAt(obs.ClosedAt)
	}
	if obs.RateLimitResetAt.IsZero() {
		return upd.ClearRateLimitResetAt()
	}
	return upd.SetRateLimitResetAt(obs.RateLimitResetAt)
}

func lastPolledMatches(old *time.Time) predicate.PullRequestState {
	if old == nil {
		return pullrequeststate.LastPolledAtIsNil()
	}
	return pullrequeststate.LastPolledAt(*old)
}

// ResetVerifyCounters zeroes poll_fail_count, first_fail_at and poll_error on
// unverified-terminal rows only (state=unknown, terminal=true) and returns how
// many it touched. Called only by the service-startup maintenance pass.
func (r *EntRepository) ResetVerifyCounters(ctx context.Context) (int, error) {
	n, err := r.Client.PullRequestState.Update().
		Where(pullrequeststate.State(string(domain.PRStateUnknown)), pullrequeststate.Terminal(true)).
		SetPollFailCount(0).ClearFirstFailAt().SetPollError("").Save(ctx)
	if err != nil {
		return 0, fmt.Errorf("reset verify counters: %w", err)
	}
	return n, nil
}

// ListPRMirrors returns the stored legacy PR columns of every session that has a
// uuid, keyed by uuid: one narrow query for the mirror reconciler.
func (r *EntRepository) ListPRMirrors(ctx context.Context) (map[string]PRMirror, error) {
	var rows []struct {
		UUID     string `json:"uuid"`
		URL      string `json:"github_pr_url"`
		Number   int    `json:"github_pr_number"`
		Terminal bool   `json:"github_pr_status_terminal"`
	}
	err := r.Client.Session.Query().Where(entsession.UUIDNEQ("")).
		Select(entsession.FieldUUID, entsession.FieldGithubPrURL, entsession.FieldGithubPrNumber, entsession.FieldGithubPrStatusTerminal).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("list pr mirrors: %w", err)
	}
	out := make(map[string]PRMirror, len(rows))
	for _, row := range rows {
		out[row.UUID] = PRMirror{URL: row.URL, Number: row.Number, Terminal: row.Terminal}
	}
	return out, nil
}

// ListLinkOwnersByStateIDs returns the distinct session uuids and item ids that
// link any of the given PullRequestState rows (one query per chunk of ids).
func (r *EntRepository) ListLinkOwnersByStateIDs(ctx context.Context, stateIDs []int) (sessionUUIDs, itemIDs []string, err error) {
	seenS, seenI := map[string]struct{}{}, map[string]struct{}{}
	for _, chunk := range chunkSlice(stateIDs, prStateIDChunk) {
		links, qerr := r.Client.SessionPR.Query().Where(sessionpr.PullRequestStateIDIn(chunk...)).All(ctx)
		if qerr != nil {
			return nil, nil, fmt.Errorf("list link owners by state: %w", qerr)
		}
		for _, l := range links {
			if l.SessionUUID != "" {
				if _, ok := seenS[l.SessionUUID]; !ok {
					seenS[l.SessionUUID] = struct{}{}
					sessionUUIDs = append(sessionUUIDs, l.SessionUUID)
				}
			}
			if l.BacklogItemID != "" {
				if _, ok := seenI[l.BacklogItemID]; !ok {
					seenI[l.BacklogItemID] = struct{}{}
					itemIDs = append(itemIDs, l.BacklogItemID)
				}
			}
		}
	}
	return sessionUUIDs, itemIDs, nil
}
