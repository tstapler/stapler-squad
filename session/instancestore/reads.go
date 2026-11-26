package instancestore

import (
	"context"
	"fmt"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/session"
	"github.com/tstapler/stapler-squad/session/ent/tag"
)

// GetClaudeConversationUUIDBySessionUUID returns the Claude conversation UUID
// for the session whose title (tmux session name) matches sessionUUID.
// Returns "" if the session has no associated ClaudeSession.
func (i *Instances) GetClaudeConversationUUIDBySessionUUID(ctx context.Context, sessionUUID string) (string, error) {
	sess, err := i.client.Session.Query().
		Where(session.Title(sessionUUID)).
		WithClaudeSession().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", domain.ErrNotFound
		}
		return "", fmt.Errorf("GetClaudeConversationUUIDBySessionUUID: %w", err)
	}
	if sess.Edges.ClaudeSession == nil {
		return "", nil
	}
	return sess.Edges.ClaudeSession.ClaudeSessionID, nil
}

// Get retrieves a single session by title
func (i *Instances) Get(ctx context.Context, title string) (*domain.InstanceData, error) {
	// Find session with all relationships eagerly loaded
	sess, err := applyLoadOptions(
		i.client.Session.Query().Where(session.Title(title)),
		domain.LoadFull,
	).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("session not found: %s", title)
		}
		return nil, fmt.Errorf("failed to query session: %w", err)
	}

	// Convert to InstanceData
	return sessionToInstanceData(sess), nil
}

// listLoadOptions mirrors the eager-loading historically done by List/ListByStatus/ListByTag
// (worktree, tags, project, claude session+metadata — but NOT diff stats, unlike Get).
var listLoadOptions = domain.LoadOptions{
	LoadWorktree:      true,
	LoadTags:          true,
	LoadClaudeSession: true,
}

// applyLoadOptions conditionally chains eager-load edges onto a session query based on
// options. WithProject() stays unconditional since domain.LoadOptions has no corresponding field
// and every existing caller expects the project edge to be populated.
func applyLoadOptions(q *ent.SessionQuery, options domain.LoadOptions) *ent.SessionQuery {
	if options.LoadWorktree {
		q = q.WithWorktree()
	}
	if options.LoadDiffStats || options.LoadDiffContent {
		q = q.WithDiffStats()
	}
	if options.LoadTags {
		q = q.WithTags()
	}
	q = q.WithProject()
	if options.LoadClaudeSession {
		q = q.WithClaudeSession(func(cq *ent.ClaudeSessionQuery) {
			cq.WithMetadata()
		})
	}
	return q
}

// List retrieves all sessions from the database
func (i *Instances) List(ctx context.Context) ([]domain.InstanceData, error) {
	return i.ListWithOptions(ctx, listLoadOptions)
}

// ListByStatus retrieves sessions filtered by status
func (i *Instances) ListByStatus(ctx context.Context, status domain.Status) ([]domain.InstanceData, error) {
	// Query sessions by status with relationships
	sessions, err := applyLoadOptions(
		i.client.Session.Query().Where(session.Status(int(status))),
		listLoadOptions,
	).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions by status: %w", err)
	}

	// Convert to InstanceData
	result := make([]domain.InstanceData, len(sessions))
	for idx, s := range sessions {
		result[idx] = *sessionToInstanceData(s)
	}

	return result, nil
}

// ListByTag retrieves sessions that have a specific tag
func (i *Instances) ListByTag(ctx context.Context, tagName string) ([]domain.InstanceData, error) {
	// Query sessions that have the specified tag
	sessions, err := applyLoadOptions(
		i.client.Session.Query().Where(session.HasTagsWith(tag.Name(tagName))),
		listLoadOptions,
	).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions by tag: %w", err)
	}

	// Convert to InstanceData
	result := make([]domain.InstanceData, len(sessions))
	for idx, s := range sessions {
		result[idx] = *sessionToInstanceData(s)
	}

	return result, nil
}

// GetWithOptions retrieves a single session with selective child data loading.
func (i *Instances) GetWithOptions(ctx context.Context, title string, options domain.LoadOptions) (*domain.InstanceData, error) {
	sess, err := applyLoadOptions(
		i.client.Session.Query().Where(session.Title(title)),
		options,
	).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("session not found: %s", title)
		}
		return nil, fmt.Errorf("failed to query session: %w", err)
	}
	return sessionToInstanceData(sess), nil
}

// FindByIDWithOptions retrieves a single session by stable UUID or title (mirroring
// InstanceData.MatchesID's dual-key lookup) via an indexed WHERE clause, with
// selective child data loading. Returns domain.ErrInstanceDataNotFound when no match exists,
// including for id == "" — uuid has no uniqueness constraint, so an empty id would
// otherwise match every session that has never had a UUID assigned.
func (i *Instances) FindByIDWithOptions(ctx context.Context, id string, options domain.LoadOptions) (*domain.InstanceData, error) {
	if id == "" {
		return nil, domain.ErrInstanceDataNotFound
	}
	sess, err := applyLoadOptions(
		i.client.Session.Query().Where(session.Or(session.Title(id), session.UUID(id))),
		options,
	).First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, domain.ErrInstanceDataNotFound
		}
		return nil, fmt.Errorf("failed to query session by id %s: %w", id, err)
	}
	return sessionToInstanceData(sess), nil
}

// ListWithOptions retrieves all sessions with selective child data loading.
func (i *Instances) ListWithOptions(ctx context.Context, options domain.LoadOptions) ([]domain.InstanceData, error) {
	//nolint:entfullscan the base "list all sessions" query genuinely needs every row; callers select fields via domain.LoadOptions, not row filtering.
	sessions, err := applyLoadOptions(i.client.Session.Query(), options).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions: %w", err)
	}

	result := make([]domain.InstanceData, len(sessions))
	for idx, s := range sessions {
		result[idx] = *sessionToInstanceData(s)
	}
	return result, nil
}

// ListByStatusWithOptions retrieves sessions filtered by status with selective loading.
func (i *Instances) ListByStatusWithOptions(ctx context.Context, status domain.Status, options domain.LoadOptions) ([]domain.InstanceData, error) {
	sessions, err := applyLoadOptions(
		i.client.Session.Query().Where(session.Status(int(status))),
		options,
	).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions by status: %w", err)
	}

	result := make([]domain.InstanceData, len(sessions))
	for idx, s := range sessions {
		result[idx] = *sessionToInstanceData(s)
	}
	return result, nil
}

// ListByTagWithOptions retrieves sessions with a specific tag with selective loading.
func (i *Instances) ListByTagWithOptions(ctx context.Context, tagName string, options domain.LoadOptions) ([]domain.InstanceData, error) {
	sessions, err := applyLoadOptions(
		i.client.Session.Query().Where(session.HasTagsWith(tag.Name(tagName))),
		options,
	).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions by tag: %w", err)
	}

	result := make([]domain.InstanceData, len(sessions))
	for idx, s := range sessions {
		result[idx] = *sessionToInstanceData(s)
	}
	return result, nil
}
