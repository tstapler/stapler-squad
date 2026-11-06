package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsWebhookManagementPath_should_MatchExactlyTheRoutesTheHandlerServes(t *testing.T) {
	mux := http.NewServeMux()
	NewWebhookRegistrationHandler(&WebhookRegistrationService{}).RegisterRoutes(mux)

	served := []struct{ method, path string }{
		{http.MethodGet, webhookManagementBasePath + "/capability"},
		{http.MethodGet, webhookManagementBasePath + "/registrations/inst-1"},
		{http.MethodPost, webhookManagementBasePath + "/registrations/inst-1/reconcile"},
		{http.MethodPost, webhookManagementBasePath + "/registrations/inst-1/disable"},
		{http.MethodPost, webhookManagementBasePath + "/registrations/inst-1/delete"},
		{http.MethodPost, webhookManagementBasePath + "/registrations/inst-1/emergency-cleanup"},
	}
	for _, r := range served {
		_, pattern := mux.Handler(httptest.NewRequest(r.method, r.path, nil))
		assert.NotEmpty(t, pattern, "%s %s should be a registered route", r.method, r.path)
		assert.True(t, IsWebhookManagementPath(r.path),
			"%s is served by the handler, so the passkey middleware must step aside for it", r.path)
	}
}

func TestIsWebhookManagementPath_should_RejectLookalikes_When_PathIsNotARealRoute(t *testing.T) {
	for name, path := range map[string]string{
		"prefix only":             webhookManagementBasePath,
		"prefix with slash":       webhookManagementBasePath + "/",
		"unknown leaf":            webhookManagementBasePath + "/admin",
		"unknown action":          webhookManagementBasePath + "/registrations/inst-1/purge",
		"no instance":             webhookManagementBasePath + "/registrations/",
		"no instance with action": webhookManagementBasePath + "/registrations//reconcile",
		"extra segment":           webhookManagementBasePath + "/registrations/a/b/reconcile",
		"trailing slash":          webhookManagementBasePath + "/capability/",
		"instance id too long":    webhookManagementBasePath + "/registrations/" + strings.Repeat("a", 65),
		"instance id with dots":   webhookManagementBasePath + "/registrations/../reconcile",
		"instance id with space":  webhookManagementBasePath + "/registrations/a b/reconcile",
		"other version":           "/api/integrations/webhooks/v2/capability",
		"different case":          "/API/integrations/webhooks/v1/capability",
		"sibling api":             "/api/session.v1.SessionService/ListSessions",
		"traversal":               webhookManagementBasePath + "/capability/../../../x",
		"empty":                   "",
	} {
		assert.False(t, IsWebhookManagementPath(path), "%s (%q) must not be treated as a self-authenticated route", name, path)
	}
}

func TestWebhookManagement_should_RejectDotSegmentInstanceIDs_When_ServiceIsCalledDirectly(t *testing.T) {
	svc := &WebhookRegistrationService{} // no repo: validation must refuse before anything is touched
	caller := &WebhookCaller{}

	for _, id := range []string{".", "..", "-leading", "_leading", ".hidden", "", strings.Repeat("a", 65), "a/b", "a b"} {
		_, err := svc.Inspect(context.Background(), caller, id)
		var apiError *WebhookAPIError
		if assert.ErrorAs(t, err, &apiError, "instance id %q", id) {
			assert.Equal(t, http.StatusBadRequest, apiError.Status, "instance id %q", id)
		}
	}
}
