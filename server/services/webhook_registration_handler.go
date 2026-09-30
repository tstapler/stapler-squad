package services

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
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
	svc *WebhookRegistrationService
}

// NewWebhookRegistrationHandler constructs the handler.
func NewWebhookRegistrationHandler(svc *WebhookRegistrationService) *WebhookRegistrationHandler {
	return &WebhookRegistrationHandler{svc: svc}
}

// RegisterRoutes registers the management routes on mux.
func (h *WebhookRegistrationHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+webhookManagementBasePath+"/capability", h.capability)
	mux.HandleFunc("GET "+webhookManagementBasePath+"/registrations/{instance_id}", h.inspect)
	mux.HandleFunc("POST "+webhookManagementBasePath+"/registrations/{instance_id}/reconcile", h.reconcile)
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
func (h *WebhookRegistrationHandler) authenticate(w http.ResponseWriter, r *http.Request) (*WebhookCaller, bool) {
	header := r.Header.Get("Authorization")
	token, found := strings.CutPrefix(header, "Bearer ")
	if caller := h.svc.Authenticate(r.Context(), token); found && caller != nil {
		return caller, true
	}
	log.Warn("[WebhookManagement] unauthenticated request", "path", r.URL.Path)
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
