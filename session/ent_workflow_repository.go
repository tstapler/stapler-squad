package session

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/workflow"
)

// EntWorkflowRepository implements WorkflowRepository using the ent ORM.
type EntWorkflowRepository struct {
	client *ent.Client
}

// NewEntWorkflowRepository creates a new ent-backed workflow repository.
func NewEntWorkflowRepository(client *ent.Client) *EntWorkflowRepository {
	return &EntWorkflowRepository{client: client}
}

// Create inserts a new workflow definition.
// Returns ent.ConstraintError when a duplicate slug exists.
func (r *EntWorkflowRepository) Create(ctx context.Context, w WorkflowCreateInput) (*ent.Workflow, error) {
	c := r.client.Workflow.Create().
		SetSlug(w.Slug).
		SetName(w.Name).
		SetCommand(w.Command).
		SetCronEnabled(w.CronEnabled)
	if w.Enabled != nil {
		c.SetEnabled(*w.Enabled)
	}

	if w.Description != "" {
		c.SetDescription(w.Description)
	}
	if w.TargetDirectory != "" {
		c.SetTargetDirectory(w.TargetDirectory)
	}
	if w.InputTemplate != "" {
		c.SetInputTemplate(w.InputTemplate)
	}
	if w.SessionType != "" {
		c.SetSessionType(w.SessionType)
	}
	if w.Model != "" {
		c.SetModel(w.Model)
	}
	if w.AgentType != "" {
		c.SetAgentType(w.AgentType)
	}
	if w.CronExpression != "" {
		c.SetCronExpression(w.CronExpression)
	}
	if w.KeepSessions != nil {
		c.SetKeepSessions(*w.KeepSessions)
	}
	if w.ArchiveAfterHours != nil {
		c.SetArchiveAfterHours(*w.ArchiveAfterHours)
	}
	if w.TriggerType != "" {
		c.SetTriggerType(w.TriggerType)
	}
	if w.GitHubRepo != "" {
		c.SetGithubRepo(w.GitHubRepo)
	}
	if w.GitHubBranch != "" {
		c.SetGithubBranch(w.GitHubBranch)
	}
	if w.WebhookSlug != "" {
		c.SetWebhookSlug(w.WebhookSlug)
	}
	if w.WebhookSecretEncrypted != "" {
		c.SetWebhookSecretEncrypted(w.WebhookSecretEncrypted)
	}
	if w.EventFilter != "" {
		c.SetEventFilter(w.EventFilter)
	}
	if w.LabelFilter != "" {
		c.SetLabelFilter(w.LabelFilter)
	}
	if w.PromptTemplate != "" {
		c.SetPromptTemplate(w.PromptTemplate)
	}

	wf, err := c.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, fmt.Errorf("%w: workflow with slug %q already exists", ErrConflict, w.Slug)
		}
		return nil, fmt.Errorf("create workflow: %w", err)
	}
	return wf, nil
}

// Update applies a partial update to an existing workflow by UUID, unconditionally.
// Thin wrapper over UpdateConditional with the zero time.Time, which applies no
// updated_at precondition — kept as a separate method so existing single-writer
// callers don't need to thread an expectedUpdatedAt they don't have.
func (r *EntWorkflowRepository) Update(ctx context.Context, id uuid.UUID, w WorkflowUpdateInput) (*ent.Workflow, error) {
	return r.UpdateConditional(ctx, id, w, time.Time{})
}

// UpdateConditional applies a partial update to an existing workflow by UUID, only if
// the row's current updated_at exactly matches expectedUpdatedAt — an optimistic-
// concurrency CAS. A zero expectedUpdatedAt applies no precondition (always writes),
// matching Update's unconditional behavior.
//
// Built on WorkflowUpdateOne (UpdateOneID), not the bulk Update().Where() builder that
// an earlier version of this method used: UpdateOneID's Save() returns the mutated
// entity directly, computed inside the same UPDATE statement/transaction ent's
// generated sqlgraph.UpdateNode issues — one atomic round trip, matching this method's
// pre-CAS performance and read-your-own-write consistency exactly when
// expectedUpdatedAt is zero (the common case: every existing single-writer caller, plus
// the hot per-fire LastFiredAt bump in Scheduler). The bulk builder's Update().Where()
// only returns an affected-row count, forcing a second, separate, unguarded Get to
// reload the entity — which both doubles the round trips on every call AND lets a
// concurrent writer's update land in the gap between the CAS write and that reload,
// so the caller could receive someone else's state as if it were their own write's
// result. UpdateOneID.Where() supports the same predicate this needs
// (workflow.UpdatedAtEQ), so there's no capability lost by using it instead.
func (r *EntWorkflowRepository) UpdateConditional(ctx context.Context, id uuid.UUID, w WorkflowUpdateInput, expectedUpdatedAt time.Time) (*ent.Workflow, error) {
	u := r.client.Workflow.UpdateOneID(id)
	if !expectedUpdatedAt.IsZero() {
		u = u.Where(workflow.UpdatedAtEQ(expectedUpdatedAt))
	}
	applyWorkflowUpdate(u, w)

	wf, err := u.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			// Either the row doesn't exist at all, or (when a precondition was
			// supplied) it exists but updated_at no longer matches — both collapse
			// to the identical "0 rows matched WHERE id=... AND updated_at=..."
			// result at the SQL level, so disambiguate with one targeted existence
			// check, paid only on this (rare) failure path, not on every call.
			if !expectedUpdatedAt.IsZero() {
				if _, getErr := r.client.Workflow.Get(ctx, id); getErr == nil {
					return nil, fmt.Errorf("%w: workflow %s updated_at mismatch", ErrPreconditionFailed, id)
				}
			}
			return nil, fmt.Errorf("%w: workflow %s", ErrNotFound, id)
		}
		if ent.IsConstraintError(err) {
			return nil, fmt.Errorf("%w: slug already exists", ErrConflict)
		}
		return nil, fmt.Errorf("update workflow %s: %w", id, err)
	}
	return wf, nil
}

// applyWorkflowUpdate copies every non-nil field from w onto u. Split out of
// UpdateConditional (rather than inlined) purely to keep that function's cyclomatic
// complexity under CI's gocyclo threshold — this is just a flat list of independent
// optional-field assignments, no branching logic of its own beyond WebhookSlug's
// clear-vs-set distinction.
func applyWorkflowUpdate(u *ent.WorkflowUpdateOne, w WorkflowUpdateInput) {
	if w.Name != nil {
		u.SetName(*w.Name)
	}
	if w.Description != nil {
		u.SetDescription(*w.Description)
	}
	if w.Command != nil {
		u.SetCommand(*w.Command)
	}
	if w.TargetDirectory != nil {
		u.SetTargetDirectory(*w.TargetDirectory)
	}
	if w.InputTemplate != nil {
		u.SetInputTemplate(*w.InputTemplate)
	}
	if w.SessionType != nil {
		u.SetSessionType(*w.SessionType)
	}
	if w.Model != nil {
		u.SetModel(*w.Model)
	}
	if w.AgentType != nil {
		u.SetAgentType(*w.AgentType)
	}
	if w.CronExpression != nil {
		u.SetCronExpression(*w.CronExpression)
	}
	if w.CronEnabled != nil {
		u.SetCronEnabled(*w.CronEnabled)
	}
	if w.Enabled != nil {
		u.SetEnabled(*w.Enabled)
	}
	if w.KeepSessions != nil {
		u.SetKeepSessions(*w.KeepSessions)
	}
	if w.ArchiveAfterHours != nil {
		u.SetArchiveAfterHours(*w.ArchiveAfterHours)
	}
	if w.TriggerType != nil {
		u.SetTriggerType(*w.TriggerType)
	}
	if w.GitHubRepo != nil {
		u.SetGithubRepo(*w.GitHubRepo)
	}
	if w.GitHubBranch != nil {
		u.SetGithubBranch(*w.GitHubBranch)
	}
	if w.WebhookSlug != nil {
		// webhook_slug is .Optional().Unique() but not .Nillable() (see
		// session/ent/schema/workflow.go), so an empty string is a real column
		// value, not NULL — two workflows both cleared to "" would collide on
		// the unique index. ClearWebhookSlug leaves the column NULL instead,
		// mirroring Create's `if w.WebhookSlug != ""` guard above.
		if *w.WebhookSlug == "" {
			u.ClearWebhookSlug()
		} else {
			u.SetWebhookSlug(*w.WebhookSlug)
		}
	}
	if w.WebhookSecretEncrypted != nil {
		u.SetWebhookSecretEncrypted(*w.WebhookSecretEncrypted)
	}
	if w.EventFilter != nil {
		u.SetEventFilter(*w.EventFilter)
	}
	if w.LabelFilter != nil {
		u.SetLabelFilter(*w.LabelFilter)
	}
	if w.PromptTemplate != nil {
		u.SetPromptTemplate(*w.PromptTemplate)
	}
	if w.LastFiredAt != nil {
		u.SetLastFiredAt(*w.LastFiredAt)
	}
}

// Delete removes a workflow by UUID.
func (r *EntWorkflowRepository) Delete(ctx context.Context, id uuid.UUID) error {
	err := r.client.Workflow.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%w: workflow %s", ErrNotFound, id)
		}
		return fmt.Errorf("delete workflow %s: %w", id, err)
	}
	return nil
}

// GetByID retrieves a workflow by UUID.
func (r *EntWorkflowRepository) GetByID(ctx context.Context, id uuid.UUID) (*ent.Workflow, error) {
	wf, err := r.client.Workflow.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: workflow %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("get workflow %s: %w", id, err)
	}
	return wf, nil
}

// GetBySlug retrieves a workflow by slug.
func (r *EntWorkflowRepository) GetBySlug(ctx context.Context, slug string) (*ent.Workflow, error) {
	wf, err := r.client.Workflow.Query().
		Where(workflow.Slug(slug)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: workflow with slug %q", ErrNotFound, slug)
		}
		return nil, fmt.Errorf("get workflow by slug %q: %w", slug, err)
	}
	return wf, nil
}

// GetByWebhookSlug retrieves a workflow by its webhook_slug.
func (r *EntWorkflowRepository) GetByWebhookSlug(ctx context.Context, slug string) (*ent.Workflow, error) {
	wf, err := r.client.Workflow.Query().
		Where(workflow.WebhookSlug(slug)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: workflow with webhook_slug %q", ErrNotFound, slug)
		}
		return nil, fmt.Errorf("get workflow by webhook_slug %q: %w", slug, err)
	}
	return wf, nil
}

// ListAll returns all workflows sorted ascending by created_at.
// A safety cap of 1000 is applied to prevent runaway queries.
func (r *EntWorkflowRepository) ListAll(ctx context.Context) ([]*ent.Workflow, error) {
	wfs, err := r.client.Workflow.Query().
		Order(ent.Asc(workflow.FieldCreatedAt)).
		Limit(1000).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all workflows: %w", err)
	}
	return wfs, nil
}

// ListEnabled returns only workflows where cron_enabled is true.
func (r *EntWorkflowRepository) ListEnabled(ctx context.Context) ([]*ent.Workflow, error) {
	wfs, err := r.client.Workflow.Query().
		Where(workflow.CronEnabled(true)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled workflows: %w", err)
	}
	return wfs, nil
}

// ListByTriggerType returns all workflows with the given trigger_type, regardless of
// cron_enabled (see interface doc comment for why enabled/repo/branch filtering is
// left to the caller).
func (r *EntWorkflowRepository) ListByTriggerType(ctx context.Context, triggerType string) ([]*ent.Workflow, error) {
	wfs, err := r.client.Workflow.Query().
		Where(workflow.TriggerType(triggerType)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workflows by trigger_type %q: %w", triggerType, err)
	}
	return wfs, nil
}
