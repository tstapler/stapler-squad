package server

import (
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

// registerWebhookManagement mounts the credential-authenticated webhook-registration API.
// Like the webhook_triggers receivers it is gated at route-registration time, so a
// disabled feature is indistinguishable from a path that never existed; flipping the flag
// needs a restart. Skipped (with a log line, never a panic) when storage is not ent-backed.
//
// cfg must be the same *config.Config instance the webhook receivers use (see the call site).
func registerWebhookManagement(srv *Server, deps *ServerDependencies, cfg *config.Config) {
	if !cfg.GetFeatureFlag(config.FeatureWebhookManagement) {
		return
	}
	if deps.Storage == nil {
		log.Warn("webhook management API not registered: no storage")
		return
	}	// Provision the machine key now, on the shared instance: a failure then disables the API
	// at boot instead of surfacing as an opaque error on the first reconcile.
	if _, err := cfg.GetOrCreateEncryptionKey(); err != nil {
		log.Warn("webhook management API not registered: cannot provision encryption key", "err", err)
		return
	}
	entClient := deps.Storage.GetEntClient()
	if entClient == nil {
		log.Warn("webhook management API not registered: storage has no ent client")
		return
	}
	configDir, err := config.GetConfigDir()
	if err != nil {
		log.Warn("webhook management API not registered: cannot resolve state directory", "err", err)
		return
	}

	receiverEnabled := cfg.GetFeatureFlag("webhook_triggers")
	svc := services.NewWebhookRegistrationService(
		session.NewEntRepositoryFromClient(entClient), cfg, services.WebhookWorkspaceID(configDir), receiverEnabled)
	svc.SetEventBus(deps.EventBus)
	services.NewWebhookRegistrationHandler(svc).RegisterRoutes(srv.mux)
	log.Info("Registered webhook management API", "base", "/api/integrations/webhooks/v1", "receiver_enabled", receiverEnabled)
	if !receiverEnabled {
		log.Warn("webhook_management is enabled but webhook_triggers is not: reconcile will refuse to create registrations until the receiver is enabled and the service restarts")
	}
}
