package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/webhookregistration"
	"github.com/tstapler/stapler-squad/session/ent/webhookrequestledger"
)

// Webhook-management persistence. Every mutation runs in one ent transaction. The repo's
// single-connection SQLite pool (SetMaxOpenConns(1)) means a transaction excludes all
// other writers, so the ledger check, version CAS and mutation below are atomic with
// respect to each other. Code inside a transaction must use only the tx client: calling
// r.client while a tx holds the one connection would deadlock.

var (
	// ErrIdempotencyKeyReused: a request ID was replayed with a different request body.
	ErrIdempotencyKeyReused = errors.New("request id reused with a different request")
	// ErrWebhookSlugUnavailable: the slug collides with a workflow this caller does not own.
	// Deliberately says nothing about who owns it.
	ErrWebhookSlugUnavailable = errors.New("webhook slug unavailable")
	// ErrCompatTupleMismatch: the request's compatibility tuple differs from the immutable
	// one persisted at creation.
	ErrCompatTupleMismatch = errors.New("compatibility tuple differs from the one persisted at creation")
	// ErrImmutableWebhookField: the request tried to change a field fixed at creation.
	ErrImmutableWebhookField = errors.New("field is immutable after creation")
	// ErrRegistrationOrphaned: the registration exists but its workflow was deleted
	// out-of-band. Reconcile refuses to guess whether that deletion was deliberate.
	ErrRegistrationOrphaned = errors.New("registration's workflow no longer exists")
)

// WebhookVersionConflictError reports a failed expected_version CAS.
type WebhookVersionConflictError struct{ CurrentVersion int64 }

func (e *WebhookVersionConflictError) Error() string {
	return fmt.Sprintf("version conflict: current version is %d", e.CurrentVersion)
}

// WebhookScope is the authenticated ownership scope every query is constrained to.
type WebhookScope struct {
	PrincipalID string
	WorkspaceID string
}

// WebhookWorkflowSpec is the desired state of the webhook-trigger workflow a registration owns.
type WebhookWorkflowSpec struct {
	WebhookSlug     string `json:"webhook_slug"`
	Name            string `json:"name"`
	Command         string `json:"command"`
	TargetDirectory string `json:"target_directory"`
	PromptTemplate  string `json:"prompt_template"`
	EventFilter     string `json:"event_filter"`
	LabelFilter     string `json:"label_filter"`
	Enabled         bool   `json:"enabled"`
}

// Reconcile outcomes.
const (
	WebhookOutcomeCreated   = "created"
	WebhookOutcomeUpdated   = "updated"
	WebhookOutcomeUnchanged = "unchanged"
)

const webhookOperationReconcile = "reconcile"

// WebhookReconcileInput is one fully validated reconcile request. SecretEncrypted and
// SecretDigest are both empty on an update that leaves the secret as it is.
type WebhookReconcileInput struct {
	Scope             WebhookScope
	RequestID         string
	Fingerprint       string
	InstanceID        string
	ExpectedVersion   int64
	CompatTuple       string
	CompatTupleSHA256 string
	SecretEncrypted   string
	SecretDigest      string
	Workflow          WebhookWorkflowSpec
}

// WebhookRegistrationView is the secret-free externally visible state of a registration.
type WebhookRegistrationView struct {
	InstanceID        string              `json:"instance_id"`
	RegistrationID    uuid.UUID           `json:"registration_id"`
	WorkflowID        uuid.UUID           `json:"workflow_id"`
	Version           int64               `json:"version"`
	CompatTupleSHA256 string              `json:"compat_tuple_sha256"`
	CreatedAt         time.Time           `json:"created_at"`
	UpdatedAt         time.Time           `json:"updated_at"`
	Workflow          WebhookWorkflowSpec `json:"workflow"`
}

// WebhookReconcileResult is what a reconcile returns, and exactly what the ledger stores
// for replay. Replayed and ChangedWorkflow are process-local and never persisted.
type WebhookReconcileResult struct {
	Outcome         string                  `json:"outcome"`
	Registration    WebhookRegistrationView `json:"registration"`
	Replayed        bool                    `json:"-"`
	ChangedWorkflow *ent.Workflow           `json:"-"`
}

// workflowSpecOf returns the zero spec for a nil workflow: a registration whose workflow was
// deleted out-of-band can still be described (and cleaned up).
func workflowSpecOf(wf *ent.Workflow) WebhookWorkflowSpec {
	if wf == nil {
		return WebhookWorkflowSpec{}
	}
	return WebhookWorkflowSpec{
		WebhookSlug:     wf.WebhookSlug,
		Name:            wf.Name,
		Command:         wf.Command,
		TargetDirectory: wf.TargetDirectory,
		PromptTemplate:  wf.PromptTemplate,
		EventFilter:     wf.EventFilter,
		LabelFilter:     wf.LabelFilter,
		Enabled:         wf.Enabled,
	}
}

func registrationView(reg *ent.WebhookRegistration, wf *ent.Workflow) WebhookRegistrationView {
	return WebhookRegistrationView{
		InstanceID:        reg.InstanceID,
		RegistrationID:    reg.ID,
		WorkflowID:        reg.WorkflowID,
		Version:           reg.Version,
		CompatTupleSHA256: reg.CompatTupleSha256,
		CreatedAt:         reg.CreatedAt,
		UpdatedAt:         reg.UpdatedAt,
		Workflow:          workflowSpecOf(wf),
	}
}

func registrationByScope(ctx context.Context, c *ent.WebhookRegistrationClient, scope WebhookScope, instanceID string) (*ent.WebhookRegistration, error) {
	reg, err := c.Query().Where(
		webhookregistration.PrincipalID(scope.PrincipalID),
		webhookregistration.WorkspaceID(scope.WorkspaceID),
		webhookregistration.InstanceID(instanceID),
	).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query webhook registration: %w", err)
	}
	return reg, nil
}

// InspectWebhookRegistration returns the caller's registration for instanceID. A
// registration owned by any other scope is indistinguishable from a missing one.
func (r *EntRepository) InspectWebhookRegistration(ctx context.Context, scope WebhookScope, instanceID string) (*WebhookRegistrationView, error) {
	reg, err := registrationByScope(ctx, r.client.WebhookRegistration, scope, instanceID)
	if err != nil {
		return nil, err
	}
	wf, err := r.client.Workflow.Get(ctx, reg.WorkflowID)
	if ent.IsNotFound(err) {
		return nil, ErrRegistrationOrphaned
	}
	if err != nil {
		return nil, fmt.Errorf("load registration workflow: %w", err)
	}
	view := registrationView(reg, wf)
	return &view, nil
}

// ReconcileWebhookRegistration applies in idempotently. See the package comment for the
// atomicity argument. On any error the transaction rolls back and nothing is written.
func (r *EntRepository) ReconcileWebhookRegistration(ctx context.Context, in WebhookReconcileInput) (*WebhookReconcileResult, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin webhook reconcile transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after a successful Commit

	key := ledgerKey{Scope: in.Scope, RequestID: in.RequestID, Fingerprint: in.Fingerprint}
	if replay, found, err := ledgerReplay(ctx, tx, key); err != nil {
		return nil, err
	} else if found {
		return replay, nil
	}

	reg, err := registrationByScope(ctx, tx.WebhookRegistration, in.Scope, in.InstanceID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("lookup webhook registration: %w", err)
	}
	var currentVersion int64
	if reg != nil {
		currentVersion = reg.Version
	}
	if in.ExpectedVersion != currentVersion {
		return nil, &WebhookVersionConflictError{CurrentVersion: currentVersion}
	}

	var result *WebhookReconcileResult
	if reg == nil {
		result, err = createWebhookRegistration(ctx, tx, in)
	} else {
		result, err = updateWebhookRegistration(ctx, tx, reg, in)
	}
	if err != nil {
		return nil, err
	}

	if err := recordLedger(ctx, tx, key, webhookOperationReconcile, result); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit webhook reconcile: %w", err)
	}
	return result, nil
}

// ledgerKey identifies one request in the ledger: the caller's scope, its request ID, and the
// fingerprint of what that request asked for.
type ledgerKey struct {
	Scope       WebhookScope
	RequestID   string
	Fingerprint string
}

// ledgerReplay reports whether key.RequestID was already applied. found is false (with a nil
// error) when it was not; a found entry with a different fingerprint is ErrIdempotencyKeyReused.
func ledgerReplay(ctx context.Context, tx *ent.Tx, key ledgerKey) (result *WebhookReconcileResult, found bool, err error) {
	in := key
	row, err := tx.WebhookRequestLedger.Query().Where(
		webhookrequestledger.PrincipalID(in.Scope.PrincipalID),
		webhookrequestledger.WorkspaceID(in.Scope.WorkspaceID),
		webhookrequestledger.RequestID(in.RequestID),
	).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("lookup request ledger: %w", err)
	}
	if row.Fingerprint != in.Fingerprint {
		return nil, false, ErrIdempotencyKeyReused
	}
	var stored WebhookReconcileResult
	if err := json.Unmarshal([]byte(row.Result), &stored); err != nil {
		return nil, false, fmt.Errorf("decode stored ledger result: %w", err)
	}
	stored.Replayed = true
	return &stored, true, nil
}

func recordLedger(ctx context.Context, tx *ent.Tx, key ledgerKey, operation string, result *WebhookReconcileResult) error {
	body, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode ledger result: %w", err)
	}
	_, err = tx.WebhookRequestLedger.Create().
		SetPrincipalID(key.Scope.PrincipalID).
		SetWorkspaceID(key.Scope.WorkspaceID).
		SetRequestID(key.RequestID).
		SetFingerprint(key.Fingerprint).
		SetOperation(operation).
		SetResult(string(body)).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("record request ledger: %w", err)
	}
	return nil
}

func createWebhookRegistration(ctx context.Context, tx *ent.Tx, in WebhookReconcileInput) (*WebhookReconcileResult, error) {
	if in.SecretEncrypted == "" || in.SecretDigest == "" {
		return nil, fmt.Errorf("a webhook secret is required to create a registration")
	}
	spec := in.Workflow
	wf, err := tx.Workflow.Create().
		SetSlug(spec.WebhookSlug).
		SetName(spec.Name).
		SetCommand(spec.Command).
		SetTargetDirectory(spec.TargetDirectory).
		SetTriggerType("webhook").
		SetWebhookSlug(spec.WebhookSlug).
		SetWebhookSecretEncrypted(in.SecretEncrypted).
		SetPromptTemplate(spec.PromptTemplate).
		SetEventFilter(spec.EventFilter).
		SetLabelFilter(spec.LabelFilter).
		SetEnabled(spec.Enabled).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, ErrWebhookSlugUnavailable
		}
		return nil, fmt.Errorf("create webhook workflow: %w", err)
	}
	reg, err := tx.WebhookRegistration.Create().
		SetPrincipalID(in.Scope.PrincipalID).
		SetWorkspaceID(in.Scope.WorkspaceID).
		SetInstanceID(in.InstanceID).
		SetWorkflowID(wf.ID).
		SetCompatTuple(in.CompatTuple).
		SetCompatTupleSha256(in.CompatTupleSHA256).
		SetSecretDigest(in.SecretDigest).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create webhook registration: %w", err)
	}
	return &WebhookReconcileResult{
		Outcome:         WebhookOutcomeCreated,
		Registration:    registrationView(reg, wf),
		ChangedWorkflow: wf,
	}, nil
}

func updateWebhookRegistration(ctx context.Context, tx *ent.Tx, reg *ent.WebhookRegistration, in WebhookReconcileInput) (*WebhookReconcileResult, error) {
	if reg.CompatTupleSha256 != in.CompatTupleSHA256 {
		return nil, ErrCompatTupleMismatch
	}
	wf, err := tx.Workflow.Get(ctx, reg.WorkflowID)
	if ent.IsNotFound(err) {
		return nil, ErrRegistrationOrphaned
	}
	if err != nil {
		return nil, fmt.Errorf("load registration workflow: %w", err)
	}
	if wf.WebhookSlug != in.Workflow.WebhookSlug {
		return nil, ErrImmutableWebhookField
	}

	secretRotated := in.SecretDigest != "" && in.SecretDigest != reg.SecretDigest
	if workflowSpecOf(wf) == in.Workflow && !secretRotated {
		return &WebhookReconcileResult{Outcome: WebhookOutcomeUnchanged, Registration: registrationView(reg, wf)}, nil
	}

	spec := in.Workflow
	upd := tx.Workflow.UpdateOneID(wf.ID).
		SetName(spec.Name).
		SetCommand(spec.Command).
		SetTargetDirectory(spec.TargetDirectory).
		SetPromptTemplate(spec.PromptTemplate).
		SetEventFilter(spec.EventFilter).
		SetLabelFilter(spec.LabelFilter).
		SetEnabled(spec.Enabled)
	regUpd := tx.WebhookRegistration.UpdateOneID(reg.ID).AddVersion(1)
	if secretRotated {
		upd.SetWebhookSecretEncrypted(in.SecretEncrypted)
		regUpd.SetSecretDigest(in.SecretDigest)
	}
	newWF, err := upd.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("update webhook workflow: %w", err)
	}
	newReg, err := regUpd.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("update webhook registration: %w", err)
	}
	return &WebhookReconcileResult{
		Outcome:         WebhookOutcomeUpdated,
		Registration:    registrationView(newReg, newWF),
		ChangedWorkflow: newWF,
	}, nil
}
