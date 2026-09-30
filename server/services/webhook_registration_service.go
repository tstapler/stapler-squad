package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/workflows"
	"github.com/tstapler/stapler-squad/session"
)

// Webhook-management contract identity. Bumping CapabilityRevision is a compatible
// contract change; a client pinned to a different revision is refused rather than
// silently reconciled against semantics it has not verified.
const (
	WebhookManagementContractVersion    = "v1"
	WebhookManagementCapabilityRevision = 1

	webhookManagementBasePath = "/api/integrations/webhooks/v1"

	minWebhookSecretBytes = 32
	maxWebhookSecretBytes = 512

	digestKeyLabel = "stapler-squad/webhook-management/digest/v1"
)

var (
	webhookInstanceIDRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
	webhookRequestIDRe  = regexp.MustCompile(`^[A-Za-z0-9._-]{8,128}$`)
	webhookLabelRe      = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
	webhookSHA256Re     = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// WebhookAPIError is the only error type the HTTP layer renders. Messages are fixed,
// caller-safe strings: they never echo secrets, tokens, or another scope's data.
type WebhookAPIError struct {
	Status         int
	Code           string
	Message        string
	CurrentVersion *int64
}

func (e *WebhookAPIError) Error() string { return e.Code + ": " + e.Message }

func apiErr(status int, code, message string) *WebhookAPIError {
	return &WebhookAPIError{Status: status, Code: code, Message: message}
}

// WebhookWorkspaceID derives this instance's stable workspace identity from its state
// directory, which already encodes every isolation mode (instance, workspace, shared).
// Credentials are bound to it, so a credential issued for one state dir is useless
// against another instance or a copied database.
func WebhookWorkspaceID(configDir string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(configDir)))
	return "ws_" + hex.EncodeToString(sum[:8])
}

// WebhookCaller is an authenticated principal and the workspace/directory bounds of its credential.
type WebhookCaller struct {
	session.WebhookScope
	AllowedDirRoot string
}

// WebhookCompat is the compatibility tuple a client attests to. Its canonical bytes are
// persisted at creation and are immutable across ordinary reconciliation.
type WebhookCompat struct {
	ContractVersion    string `json:"contract_version"`
	CapabilityRevision int    `json:"capability_revision"`
	ManifestSHA256     string `json:"manifest_sha256"`
	SignerKeyID        string `json:"signer_key_id"`
	ClientName         string `json:"client_name"`
	ClientVersion      string `json:"client_version"`
}

// WebhookSpecBody is the desired webhook-trigger workflow. Enabled defaults to true.
type WebhookSpecBody struct {
	Slug            string `json:"slug"`
	Name            string `json:"name"`
	Command         string `json:"command"`
	TargetDirectory string `json:"target_directory"`
	PromptTemplate  string `json:"prompt_template"`
	EventFilter     string `json:"event_filter"`
	LabelFilter     string `json:"label_filter"`
	Enabled         *bool  `json:"enabled,omitempty"`
}

// WebhookReconcileRequest is the reconcile request body. Secret is write-only: it is
// accepted here and never returned, stored in plaintext, or logged.
type WebhookReconcileRequest struct {
	RequestID       string          `json:"request_id"`
	ExpectedVersion int64           `json:"expected_version"`
	Compat          WebhookCompat   `json:"compat"`
	Webhook         WebhookSpecBody `json:"webhook"`
	Secret          string          `json:"secret,omitempty"`
}

// String and GoString redact the secret so an accidental %v/%+v/%#v never leaks it.
func (r WebhookReconcileRequest) String() string {
	return fmt.Sprintf("WebhookReconcileRequest{request_id=%s secret=[redacted]}", r.RequestID)
}
func (r WebhookReconcileRequest) GoString() string { return r.String() }

// WebhookRegistrationBody is the secret-free registration state returned to callers.
type WebhookRegistrationBody struct {
	InstanceID        string          `json:"instance_id"`
	RegistrationID    string          `json:"registration_id"`
	WorkflowID        string          `json:"workflow_id"`
	Version           int64           `json:"version"`
	CompatTupleSHA256 string          `json:"compat_tuple_sha256"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	EndpointPath      string          `json:"endpoint_path"`
	Webhook           WebhookSpecBody `json:"webhook"`
}

// WebhookReconcileResponse is the reconcile result.
type WebhookReconcileResponse struct {
	Outcome      string                  `json:"outcome"`
	Replayed     bool                    `json:"replayed"`
	Registration WebhookRegistrationBody `json:"registration"`
}

// WebhookCapabilityResponse advertises what this server can do so a client can fail
// closed before it resolves any secret.
type WebhookCapabilityResponse struct {
	ContractVersion    string   `json:"contract_version"`
	CapabilityRevision int      `json:"capability_revision"`
	ReceiverEnabled    bool     `json:"receiver_enabled"`
	Operations         []string `json:"operations"`
}

// WebhookRegistrationService owns webhook-management request validation and the mapping
// between the API contract and persistence. Concrete type: one implementation.
type WebhookRegistrationService struct {
	repo            *session.EntRepository
	cfg             *config.Config
	workspaceID     string
	receiverEnabled bool
	eventBus        *events.EventBus

	supportedRevisions []int
}

// NewWebhookRegistrationService constructs the service. receiverEnabled must reflect
// whether the POST /webhooks/{slug} receiver is actually registered in this process:
// reconcile refuses to create endpoints nothing is listening on.
func NewWebhookRegistrationService(repo *session.EntRepository, cfg *config.Config, workspaceID string, receiverEnabled bool) *WebhookRegistrationService {
	return &WebhookRegistrationService{
		repo: repo, cfg: cfg, workspaceID: workspaceID, receiverEnabled: receiverEnabled,
		supportedRevisions: []int{WebhookManagementCapabilityRevision},
	}
}

// SetEventBus wires the bus so created/updated workflows appear live in open UIs.
func (s *WebhookRegistrationService) SetEventBus(bus *events.EventBus) { s.eventBus = bus }

// WorkspaceID is the workspace identity every credential must be bound to.
func (s *WebhookRegistrationService) WorkspaceID() string { return s.workspaceID }

// Authenticate resolves a bearer token to a caller, or nil. A credential bound to a
// different workspace is treated exactly like an unknown one.
func (s *WebhookRegistrationService) Authenticate(ctx context.Context, token string) *WebhookCaller {
	info, err := s.repo.AuthenticateIntegrationToken(ctx, token)
	if err != nil || info.WorkspaceID != s.workspaceID {
		return nil
	}
	return &WebhookCaller{
		WebhookScope:   session.WebhookScope{PrincipalID: info.PrincipalID, WorkspaceID: info.WorkspaceID},
		AllowedDirRoot: info.AllowedDirRoot,
	}
}

// Capability reports the contract identity and receiver availability.
func (s *WebhookRegistrationService) Capability() WebhookCapabilityResponse {
	return WebhookCapabilityResponse{
		ContractVersion:    WebhookManagementContractVersion,
		CapabilityRevision: WebhookManagementCapabilityRevision,
		ReceiverEnabled:    s.receiverEnabled,
		Operations:         []string{"capability", "inspect", "reconcile", "disable", "delete", "emergency_cleanup"},
	}
}

// Inspect returns the caller's registration for instanceID.
func (s *WebhookRegistrationService) Inspect(ctx context.Context, caller *WebhookCaller, instanceID string) (*WebhookRegistrationBody, error) {
	if !webhookInstanceIDRe.MatchString(instanceID) {
		return nil, apiErr(http.StatusBadRequest, "INVALID_REQUEST", "invalid instance id")
	}
	view, err := s.repo.InspectWebhookRegistration(ctx, caller.WebhookScope, instanceID)
	if err != nil {
		return nil, mapWebhookRepoError(err)
	}
	body := registrationBodyOf(view)
	return &body, nil
}

// Reconcile validates req and applies it idempotently for the caller's scope.
func (s *WebhookRegistrationService) Reconcile(ctx context.Context, caller *WebhookCaller, instanceID string, req WebhookReconcileRequest) (*WebhookReconcileResponse, error) {
	if !s.receiverEnabled {
		return nil, apiErr(http.StatusConflict, "RECEIVER_DISABLED", "the webhook receiver is not enabled on this server")
	}
	in, err := s.buildReconcileInput(caller, instanceID, req)
	if err != nil {
		return nil, err
	}
	result, err := s.repo.ReconcileWebhookRegistration(ctx, *in)
	if err != nil {
		mapped := mapWebhookRepoError(err)
		log.Warn("[WebhookManagement] reconcile refused", "principal", caller.PrincipalID,
			"instance", instanceID, "request_id", req.RequestID, "code", mapped.Code)
		return nil, mapped
	}
	s.publish(result)
	log.Info("[WebhookManagement] reconcile", "principal", caller.PrincipalID, "instance", instanceID,
		"request_id", req.RequestID, "outcome", result.Outcome, "replayed", result.Replayed,
		"workflow_id", result.Registration.WorkflowID.String())
	return &WebhookReconcileResponse{
		Outcome:      result.Outcome,
		Replayed:     result.Replayed,
		Registration: registrationBodyOf(&result.Registration),
	}, nil
}

func (s *WebhookRegistrationService) publish(result *session.WebhookReconcileResult) {
	if s.eventBus == nil || result.ChangedWorkflow == nil || result.Replayed {
		return
	}
	kind := events.WorkflowChangeUpdated
	if result.Outcome == session.WebhookOutcomeCreated {
		kind = events.WorkflowChangeCreated
	}
	s.eventBus.Publish(events.NewWorkflowChangedEvent(&events.WorkflowEventPayload{Kind: kind, Workflow: result.ChangedWorkflow}))
}

func (s *WebhookRegistrationService) buildReconcileInput(caller *WebhookCaller, instanceID string, req WebhookReconcileRequest) (*session.WebhookReconcileInput, error) {
	if !webhookInstanceIDRe.MatchString(instanceID) {
		return nil, apiErr(http.StatusBadRequest, "INVALID_REQUEST", "invalid instance id")
	}
	if !webhookRequestIDRe.MatchString(req.RequestID) {
		return nil, apiErr(http.StatusBadRequest, "INVALID_REQUEST", "request_id must be 8-128 characters of [A-Za-z0-9._-]")
	}
	if req.ExpectedVersion < 0 {
		return nil, apiErr(http.StatusBadRequest, "INVALID_REQUEST", "expected_version must be >= 0")
	}
	tuple, tupleSHA, err := s.canonicalCompat(req.Compat, true)
	if err != nil {
		return nil, err
	}
	spec, err := validateWebhookSpec(req.Webhook, caller.AllowedDirRoot)
	if err != nil {
		return nil, err
	}
	if req.ExpectedVersion == 0 && req.Secret == "" {
		return nil, apiErr(http.StatusBadRequest, "INVALID_REQUEST", "secret is required to create a registration")
	}
	if req.Secret != "" && (len(req.Secret) < minWebhookSecretBytes || len(req.Secret) > maxWebhookSecretBytes) {
		return nil, apiErr(http.StatusBadRequest, "WEAK_SECRET",
			fmt.Sprintf("secret must be %d-%d bytes", minWebhookSecretBytes, maxWebhookSecretBytes))
	}

	encKey, macKey, err := s.secretKeys()
	if err != nil {
		return nil, apiErr(http.StatusInternalServerError, "INTERNAL", "secret storage unavailable")
	}
	in := &session.WebhookReconcileInput{
		Scope:             caller.WebhookScope,
		RequestID:         req.RequestID,
		InstanceID:        instanceID,
		ExpectedVersion:   req.ExpectedVersion,
		CompatTuple:       tuple,
		CompatTupleSHA256: tupleSHA,
		Workflow:          spec,
	}
	if req.Secret != "" {
		encrypted, encErr := session.EncryptToken(encKey, req.Secret)
		if encErr != nil {
			return nil, apiErr(http.StatusInternalServerError, "INTERNAL", "secret storage unavailable")
		}
		in.SecretEncrypted = encrypted
		in.SecretDigest = keyedHex(macKey, []byte(req.Secret))
	}
	in.Fingerprint, err = requestFingerprint(macKey, in)
	if err != nil {
		return nil, apiErr(http.StatusInternalServerError, "INTERNAL", "could not fingerprint request")
	}
	return in, nil
}

// secretKeys returns the AES key used for storage and a domain-separated HMAC key used
// for digests and fingerprints, so one machine key is never used for two purposes directly.
func (s *WebhookRegistrationService) secretKeys() (encKey, macKey []byte, err error) {
	encKey, err = s.cfg.GetOrCreateEncryptionKey()
	if err != nil {
		return nil, nil, fmt.Errorf("get encryption key: %w", err)
	}
	return encKey, hmacBytes(encKey, []byte(digestKeyLabel)), nil
}

func hmacBytes(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data) // hash.Hash.Write never returns an error
	return mac.Sum(nil)
}

func keyedHex(key, data []byte) string { return hex.EncodeToString(hmacBytes(key, data)) }

// requestFingerprint is a keyed digest of everything that defines the request except its
// ID. The plaintext secret never enters it: only its keyed digest does.
func requestFingerprint(macKey []byte, in *session.WebhookReconcileInput) (string, error) {
	canonical, err := json.Marshal(struct {
		InstanceID        string                      `json:"instance_id"`
		ExpectedVersion   int64                       `json:"expected_version"`
		CompatTupleSHA256 string                      `json:"compat_tuple_sha256"`
		Workflow          session.WebhookWorkflowSpec `json:"workflow"`
		SecretDigest      string                      `json:"secret_digest"`
	}{in.InstanceID, in.ExpectedVersion, in.CompatTupleSHA256, in.Workflow, in.SecretDigest})
	if err != nil {
		return "", fmt.Errorf("marshal fingerprint input: %w", err)
	}
	return keyedHex(macKey, canonical), nil
}

// SetSupportedCapabilityRevisions replaces the set of capability revisions ordinary operations
// accept. Dropping a revision "revokes" it: ordinary reconcile/disable/delete under it are
// refused, while emergency cleanup of registrations created under it still works.
func (s *WebhookRegistrationService) SetSupportedCapabilityRevisions(revisions ...int) {
	s.supportedRevisions = revisions
}

func (s *WebhookRegistrationService) revisionSupported(rev int) bool {
	for _, r := range s.supportedRevisions {
		if r == rev {
			return true
		}
	}
	return false
}

// canonicalCompat validates the tuple and returns its canonical JSON (fixed field order)
// and that JSON's SHA-256: the exact bytes persisted as provenance. With enforceCurrent it
// also requires the tuple to name a contract and capability revision this server currently
// supports; emergency cleanup passes false, since its whole purpose is to act on a
// registration whose creation-time revision may since have been revoked. It then only
// validates the tuple's shape, and the caller proves it equals the persisted one.
func (s *WebhookRegistrationService) canonicalCompat(c WebhookCompat, enforceCurrent bool) (canonical, sha string, err error) {
	switch {
	case enforceCurrent && c.ContractVersion != WebhookManagementContractVersion:
		return "", "", apiErr(http.StatusConflict, "UNSUPPORTED_CONTRACT", "unsupported contract_version")
	case enforceCurrent && !s.revisionSupported(c.CapabilityRevision):
		return "", "", apiErr(http.StatusConflict, "UNSUPPORTED_CAPABILITY", "unsupported capability_revision")
	case !webhookLabelRe.MatchString(c.ContractVersion), c.CapabilityRevision < 1:
		return "", "", apiErr(http.StatusBadRequest, "INVALID_REQUEST", "compat contract_version and capability_revision are required")
	case !webhookSHA256Re.MatchString(c.ManifestSHA256):
		return "", "", apiErr(http.StatusBadRequest, "INVALID_REQUEST", "manifest_sha256 must be 64 lowercase hex characters")
	case !webhookLabelRe.MatchString(c.SignerKeyID), !webhookLabelRe.MatchString(c.ClientName), !webhookLabelRe.MatchString(c.ClientVersion):
		return "", "", apiErr(http.StatusBadRequest, "INVALID_REQUEST", "signer_key_id, client_name and client_version are required")
	}
	raw, marshalErr := json.Marshal(c)
	if marshalErr != nil {
		return "", "", apiErr(http.StatusInternalServerError, "INTERNAL", "could not encode compatibility tuple")
	}
	sum := sha256.Sum256(raw)
	return string(raw), hex.EncodeToString(sum[:]), nil
}

const (
	maxWebhookNameBytes     = 200
	maxWebhookCommandBytes  = 4096
	maxWebhookTemplateBytes = 16 << 10
	maxWebhookFilterBytes   = 256
)

func validateWebhookSpec(b WebhookSpecBody, allowedRoot string) (session.WebhookWorkflowSpec, error) {
	invalid := func(msg string) (session.WebhookWorkflowSpec, error) {
		return session.WebhookWorkflowSpec{}, apiErr(http.StatusBadRequest, "INVALID_REQUEST", msg)
	}
	if err := session.ValidateWorkflowSlug(b.Slug); err != nil {
		return invalid("webhook.slug: " + err.Error())
	}
	if b.Name == "" || len(b.Name) > maxWebhookNameBytes {
		return invalid("webhook.name is required (max 200 bytes)")
	}
	if b.Command == "" || len(b.Command) > maxWebhookCommandBytes {
		return invalid("webhook.command is required (max 4096 bytes)")
	}
	if len(b.PromptTemplate) > maxWebhookTemplateBytes || len(b.EventFilter) > maxWebhookFilterBytes || len(b.LabelFilter) > maxWebhookFilterBytes {
		return invalid("webhook field exceeds its size limit")
	}
	if b.PromptTemplate != "" {
		if err := workflows.ValidatePromptTemplate(b.PromptTemplate); err != nil {
			return invalid("webhook.prompt_template: " + err.Error())
		}
	}
	if err := validateTargetDirectory(b.TargetDirectory); err != nil {
		return invalid("webhook." + err.Error())
	}
	if !dirRoot(allowedRoot).contains(b.TargetDirectory) {
		return session.WebhookWorkflowSpec{}, apiErr(http.StatusForbidden, "FORBIDDEN_DIRECTORY", "target_directory is outside this credential's allowed directory")
	}
	enabled := true
	if b.Enabled != nil {
		enabled = *b.Enabled
	}
	return session.WebhookWorkflowSpec{
		WebhookSlug: b.Slug, Name: b.Name, Command: b.Command, TargetDirectory: b.TargetDirectory,
		PromptTemplate: b.PromptTemplate, EventFilter: b.EventFilter, LabelFilter: b.LabelFilter, Enabled: enabled,
	}, nil
}

// dirRoot is the absolute, clean directory bounding where a credential's registrations may
// point. A distinct type (not a bare string) so dir/root can never be transposed in the
// containment check without a compile error.
type dirRoot string

// contains reports whether dir is the root or beneath it. dir must already be absolute and
// clean. Symlinks are resolved when the paths exist; a directory created later as a
// symlink out of the root is not detected here.
func (r dirRoot) contains(dir string) bool {
	root := string(r)
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func registrationBodyOf(v *session.WebhookRegistrationView) WebhookRegistrationBody {
	enabled := v.Workflow.Enabled
	return WebhookRegistrationBody{
		InstanceID:        v.InstanceID,
		RegistrationID:    v.RegistrationID.String(),
		WorkflowID:        v.WorkflowID.String(),
		Version:           v.Version,
		CompatTupleSHA256: v.CompatTupleSHA256,
		CreatedAt:         v.CreatedAt,
		UpdatedAt:         v.UpdatedAt,
		EndpointPath:      "/webhooks/" + v.Workflow.WebhookSlug,
		Webhook: WebhookSpecBody{
			Slug: v.Workflow.WebhookSlug, Name: v.Workflow.Name, Command: v.Workflow.Command,
			TargetDirectory: v.Workflow.TargetDirectory, PromptTemplate: v.Workflow.PromptTemplate,
			EventFilter: v.Workflow.EventFilter, LabelFilter: v.Workflow.LabelFilter, Enabled: &enabled,
		},
	}
}

func mapWebhookRepoError(err error) *WebhookAPIError {
	var conflict *session.WebhookVersionConflictError
	switch {
	case errors.As(err, &conflict):
		e := apiErr(http.StatusConflict, "VERSION_CONFLICT", "expected_version does not match the current version")
		e.CurrentVersion = &conflict.CurrentVersion
		return e
	case errors.Is(err, session.ErrNotFound):
		return apiErr(http.StatusNotFound, "NOT_FOUND", "registration not found")
	case errors.Is(err, session.ErrIdempotencyKeyReused):
		return apiErr(http.StatusConflict, "IDEMPOTENCY_KEY_REUSED", "request_id was already used for a different request")
	case errors.Is(err, session.ErrWebhookSlugUnavailable):
		return apiErr(http.StatusConflict, "SLUG_UNAVAILABLE", "webhook slug is not available")
	case errors.Is(err, session.ErrCompatTupleMismatch):
		return apiErr(http.StatusConflict, "COMPAT_TUPLE_MISMATCH", "compatibility tuple differs from the one persisted at creation")
	case errors.Is(err, session.ErrImmutableWebhookField):
		return apiErr(http.StatusConflict, "IMMUTABLE_FIELD", "webhook.slug cannot change after creation")
	case errors.Is(err, session.ErrRegistrationOrphaned):
		return apiErr(http.StatusConflict, "REGISTRATION_ORPHANED", "the registration's workflow no longer exists")
	default:
		log.Error("[WebhookManagement] unexpected error", "err", err)
		return apiErr(http.StatusInternalServerError, "INTERNAL", "internal error")
	}
}
