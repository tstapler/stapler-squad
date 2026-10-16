package services

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/tstapler/stapler-squad/log"
)

// maxWebhookManagementBodyBytes bounds a management request body. Far smaller than the
// inbound-webhook limit: a reconcile request is a few KiB of settings, never a payload.
const maxWebhookManagementBodyBytes = 64 << 10

// WebhookRegistrationHandler serves the webhook-management HTTP API. Every route requires
// a bearer integration credential validated here, independent of the passkey middleware,
// so it is authenticated on whichever listener serves it.
type WebhookRegistrationHandler struct {
	svc     *WebhookRegistrationService
	limiter *authFailureLimiter
}

// NewWebhookRegistrationHandler constructs the handler with its own authentication-failure
// limiter (per handler, so there is no process-global state).
func NewWebhookRegistrationHandler(svc *WebhookRegistrationService) *WebhookRegistrationHandler {
	return &WebhookRegistrationHandler{svc: svc, limiter: newAuthFailureLimiter()}
}

// RegisterRoutes registers the management routes on mux.
func (h *WebhookRegistrationHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+webhookManagementBasePath+"/capability", h.capability)
	mux.HandleFunc("GET "+webhookManagementBasePath+"/registrations/{instance_id}", h.inspect)
	mux.HandleFunc("POST "+webhookManagementBasePath+"/registrations/{instance_id}/reconcile", h.reconcile)
	mux.HandleFunc("POST "+webhookManagementBasePath+"/registrations/{instance_id}/disable", h.lifecycle(
		func(ctx context.Context, c *WebhookCaller, id string, r WebhookLifecycleRequest) (*WebhookReconcileResponse, error) {
			return h.svc.Disable(ctx, c, id, r)
		}))
	mux.HandleFunc("POST "+webhookManagementBasePath+"/registrations/{instance_id}/delete", h.lifecycle(
		func(ctx context.Context, c *WebhookCaller, id string, r WebhookLifecycleRequest) (*WebhookReconcileResponse, error) {
			return h.svc.Delete(ctx, c, id, r)
		}))
	mux.HandleFunc("POST "+webhookManagementBasePath+"/registrations/{instance_id}/emergency-cleanup", h.emergencyCleanup)
}

// webhookManagementRouteRe matches exactly the routes RegisterRoutes serves, and nothing else
// under the versioned prefix. It is what lets the passkey middleware step aside for these
// paths on the remote listener: the exemption covers the real route shapes, not a whole prefix.
var webhookManagementRouteRe = regexp.MustCompile(
	`^` + regexp.QuoteMeta(webhookManagementBasePath) +
		`/(?:capability|registrations/` + webhookInstanceIDPattern + `(?:/(?:reconcile|disable|delete|emergency-cleanup))?)$`)

// IsWebhookManagementPath reports whether path is one of the management API's routes. Those
// routes authenticate every request themselves, with an integration credential, so the passkey
// middleware need not (and on the remote listener, cannot) authenticate them. The caller must
// pass only canonical paths.
func IsWebhookManagementPath(path string) bool {
	return webhookManagementRouteRe.MatchString(path)
}

type lifecycleFunc func(context.Context, *WebhookCaller, string, WebhookLifecycleRequest) (*WebhookReconcileResponse, error)

// lifecycle adapts a disable/delete service call to an HTTP handler.
func (h *WebhookRegistrationHandler) lifecycle(call lifecycleFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := h.authenticate(w, r)
		if !ok {
			return
		}
		var req WebhookLifecycleRequest
		if apiError := decodeWebhookBody(w, r, &req); apiError != nil {
			writeWebhookError(w, apiError)
			return
		}
		resp, err := call(r.Context(), caller, r.PathValue("instance_id"), req)
		if err != nil {
			writeWebhookError(w, err)
			return
		}
		writeWebhookJSON(w, http.StatusOK, resp)
	}
}

func (h *WebhookRegistrationHandler) emergencyCleanup(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	var req WebhookEmergencyRequest
	if apiError := decodeWebhookBody(w, r, &req); apiError != nil {
		writeWebhookError(w, apiError)
		return
	}
	resp, err := h.svc.EmergencyCleanup(r.Context(), caller, r.PathValue("instance_id"), req)
	if err != nil {
		writeWebhookError(w, err)
		return
	}
	writeWebhookJSON(w, http.StatusOK, resp)
}

func (h *WebhookRegistrationHandler) capability(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authenticate(w, r); !ok {
		return
	}
	writeWebhookJSON(w, http.StatusOK, h.svc.Capability())
}

func (h *WebhookRegistrationHandler) inspect(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	body, err := h.svc.Inspect(r.Context(), caller, r.PathValue("instance_id"))
	if err != nil {
		writeWebhookError(w, err)
		return
	}
	writeWebhookJSON(w, http.StatusOK, body)
}

func (h *WebhookRegistrationHandler) reconcile(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	var req WebhookReconcileRequest
	if apiError := decodeWebhookBody(w, r, &req); apiError != nil {
		writeWebhookError(w, apiError)
		return
	}
	resp, err := h.svc.Reconcile(r.Context(), caller, r.PathValue("instance_id"), req)
	if err != nil {
		writeWebhookError(w, err)
		return
	}
	status := http.StatusOK
	if resp.Outcome == "created" && !resp.Replayed {
		status = http.StatusCreated
	}
	writeWebhookJSON(w, status, resp)
}

// authenticate resolves the bearer token. Every failure mode (missing header, malformed,
// unknown, revoked, wrong workspace) returns the same response so a prober learns nothing.
//
// The failure limiter is consulted first, before the token is looked up, so a client that has
// exhausted its budget is refused with 429 even if it then presents a valid token.
func (h *WebhookRegistrationHandler) authenticate(w http.ResponseWriter, r *http.Request) (*WebhookCaller, bool) {
	key := clientLimiterKey(r.RemoteAddr)
	if retryAfter, blocked := h.limiter.blocked(key); blocked {
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
		writeWebhookError(w, apiErr(http.StatusTooManyRequests, "RATE_LIMITED", "too many failed authentication attempts"))
		return nil, false
	}
	header := r.Header.Get("Authorization")
	token, found := strings.CutPrefix(header, "Bearer ")
	if caller := h.svc.Authenticate(r.Context(), token); found && caller != nil {
		return caller, true
	}
	if h.limiter.recordFailure(key) {
		log.Warn("[WebhookManagement] authentication failure limit reached; client blocked for the rest of the window",
			"client", key, "path", r.URL.Path)
	}
	w.Header().Set("WWW-Authenticate", `Bearer realm="stapler-squad-integrations"`)
	writeWebhookError(w, apiErr(http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required"))
	return nil, false
}

// decodeWebhookBody strictly decodes a single JSON object: bounded size, JSON content type,
// no unknown fields (a typo'd field must not be silently ignored), no trailing data.
func decodeWebhookBody(w http.ResponseWriter, r *http.Request, dst any) *WebhookAPIError {
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		return apiErr(http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxWebhookManagementBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return apiErr(http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request body too large")
		}
		return apiErr(http.StatusBadRequest, "INVALID_REQUEST", "malformed request body")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return apiErr(http.StatusBadRequest, "INVALID_REQUEST", "unexpected data after request body")
	}
	return nil
}

func writeWebhookJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Warn("[WebhookManagement] failed to write response", "err", err)
	}
}

func writeWebhookError(w http.ResponseWriter, err error) {
	var apiError *WebhookAPIError
	if !errors.As(err, &apiError) {
		apiError = apiErr(http.StatusInternalServerError, "INTERNAL", "internal error")
	}
	body := struct {
		Error struct {
			Code           string `json:"code"`
			Message        string `json:"message"`
			CurrentVersion *int64 `json:"current_version,omitempty"`
		} `json:"error"`
	}{}
	body.Error.Code, body.Error.Message, body.Error.CurrentVersion = apiError.Code, apiError.Message, apiError.CurrentVersion
	writeWebhookJSON(w, apiError.Status, body)
}
