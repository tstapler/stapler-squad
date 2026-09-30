package session

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/tstapler/stapler-squad/session/ent"
)

// Lifecycle outcomes beyond reconcile's. disable on an already-disabled registration is
// WebhookOutcomeUnchanged, so a retry that lost its response is safe.
const (
	WebhookOutcomeDisabled = "disabled"
	WebhookOutcomeDeleted  = "deleted"
)

// WebhookLifecycleAction is what a lifecycle request does to a registration.
type WebhookLifecycleAction string

const (
	WebhookActionDisable WebhookLifecycleAction = "disable"
	WebhookActionDelete  WebhookLifecycleAction = "delete"
)

// WebhookLifecycleInput is one fully validated disable/delete request. Emergency marks the
// emergency-cleanup path, which differs from the ordinary one only in what the service
// checked before calling (it skips the current-capability check); this layer applies the
// same provenance, scope and CAS rules to both.
type WebhookLifecycleInput struct {
	Scope             WebhookScope
	RequestID         string
	Fingerprint       string
	InstanceID        string
	Action            WebhookLifecycleAction
	Emergency         bool
	ExpectedVersion   int64
	CompatTuple       string
	CompatTupleSHA256 string
}

func (in WebhookLifecycleInput) operation() string {
	if in.Emergency {
		return "emergency_" + string(in.Action)
	}
	return string(in.Action)
}

// ApplyWebhookLifecycle disables or deletes the caller's registration, atomically with its
// request-ID ledger entry. Order matters: replay is answered first (a completed request
// always returns its stored result), then ownership by scope, then provenance, and only
// then the version CAS, so a request that fails provenance learns nothing about the version.
// Any error rolls back and nothing is written.
func (r *EntRepository) ApplyWebhookLifecycle(ctx context.Context, in WebhookLifecycleInput) (*WebhookReconcileResult, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin webhook lifecycle transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after a successful Commit

	key := ledgerKey{Scope: in.Scope, RequestID: in.RequestID, Fingerprint: in.Fingerprint}
	if replay, found, err := ledgerReplay(ctx, tx, key); err != nil {
		return nil, err
	} else if found {
		return replay, nil
	}

	reg, err := registrationByScope(ctx, tx.WebhookRegistration, in.Scope, in.InstanceID)
	if err != nil {
		return nil, err
	}
	if !provenanceMatches(reg, in) {
		return nil, ErrCompatTupleMismatch
	}
	if in.ExpectedVersion != reg.Version {
		return nil, &WebhookVersionConflictError{CurrentVersion: reg.Version}
	}

	var result *WebhookReconcileResult
	switch in.Action {
	case WebhookActionDisable:
		result, err = disableWebhookRegistration(ctx, tx, reg)
	case WebhookActionDelete:
		result, err = deleteWebhookRegistration(ctx, tx, reg)
	default:
		return nil, fmt.Errorf("unknown webhook lifecycle action %q", in.Action)
	}
	if err != nil {
		return nil, err
	}
	if err := recordLedger(ctx, tx, key, in.operation(), result); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit webhook lifecycle: %w", err)
	}
	return result, nil
}

// provenanceMatches requires the request's tuple to equal, byte for byte and by hash, the
// tuple persisted at creation, and requires the persisted tuple to still hash to its stored
// digest. A corrupted row therefore fails closed instead of authorising anything.
func provenanceMatches(reg *ent.WebhookRegistration, in WebhookLifecycleInput) bool {
	sum := sha256.Sum256([]byte(reg.CompatTuple))
	stored := hex.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(stored), []byte(reg.CompatTupleSha256)) == 1 &&
		in.CompatTuple == reg.CompatTuple &&
		in.CompatTupleSHA256 == reg.CompatTupleSha256
}

func disableWebhookRegistration(ctx context.Context, tx *ent.Tx, reg *ent.WebhookRegistration) (*WebhookReconcileResult, error) {
	wf, err := tx.Workflow.Get(ctx, reg.WorkflowID)
	if ent.IsNotFound(err) {
		return nil, ErrRegistrationOrphaned
	}
	if err != nil {
		return nil, fmt.Errorf("load registration workflow: %w", err)
	}
	if !wf.Enabled {
		return &WebhookReconcileResult{Outcome: WebhookOutcomeUnchanged, Registration: registrationView(reg, wf)}, nil
	}
	newWF, err := tx.Workflow.UpdateOneID(wf.ID).SetEnabled(false).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("disable webhook workflow: %w", err)
	}
	newReg, err := tx.WebhookRegistration.UpdateOneID(reg.ID).AddVersion(1).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("bump webhook registration version: %w", err)
	}
	return &WebhookReconcileResult{
		Outcome:         WebhookOutcomeDisabled,
		Registration:    registrationView(newReg, newWF),
		ChangedWorkflow: newWF,
	}, nil
}

// deleteWebhookRegistration removes the registration and the workflow it owns. A workflow
// already deleted out-of-band is tolerated, so this also cleans up an orphaned registration.
// The returned view is the state immediately before deletion; the ledger keeps it for replay.
func deleteWebhookRegistration(ctx context.Context, tx *ent.Tx, reg *ent.WebhookRegistration) (*WebhookReconcileResult, error) {
	wf, err := tx.Workflow.Get(ctx, reg.WorkflowID)
	if err != nil && !ent.IsNotFound(err) {
		return nil, fmt.Errorf("load registration workflow: %w", err)
	}
	view := registrationView(reg, wf)
	if wf != nil {
		if err := tx.Workflow.DeleteOneID(wf.ID).Exec(ctx); err != nil && !errors.Is(err, ErrNotFound) && !ent.IsNotFound(err) {
			return nil, fmt.Errorf("delete webhook workflow: %w", err)
		}
	}
	if err := tx.WebhookRegistration.DeleteOneID(reg.ID).Exec(ctx); err != nil {
		return nil, fmt.Errorf("delete webhook registration: %w", err)
	}
	return &WebhookReconcileResult{Outcome: WebhookOutcomeDeleted, Registration: view}, nil
}
