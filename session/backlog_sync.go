package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent"
)

// defaultSyncInterval is the time between sync ticks.
const defaultSyncInterval = 15 * time.Minute

// SyncLoop drives periodic sync of all enabled ItemSources.
type SyncLoop struct {
	storage   *Storage
	registry  *PluginRegistry
	interval  time.Duration
	stopCh    chan struct{}
	keyFunc   func() ([]byte, error) // provides encryption key for decryption
}

// NewSyncLoop creates a SyncLoop with the default interval and no key provider.
func NewSyncLoop(storage *Storage, registry *PluginRegistry) *SyncLoop {
	return &SyncLoop{
		storage:  storage,
		registry: registry,
		interval: defaultSyncInterval,
		stopCh:   make(chan struct{}),
		keyFunc:  nil,
	}
}

// NewSyncLoopWithKeyProvider creates a SyncLoop with a key provider for decryption.
func NewSyncLoopWithKeyProvider(storage *Storage, registry *PluginRegistry, keyFunc func() ([]byte, error)) *SyncLoop {
	return &SyncLoop{
		storage:  storage,
		registry: registry,
		interval: defaultSyncInterval,
		stopCh:   make(chan struct{}),
		keyFunc:  keyFunc,
	}
}

// Start runs the sync loop until ctx is cancelled or Stop is called.
func (sl *SyncLoop) Start(ctx context.Context) {
	ticker := time.NewTicker(sl.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-sl.stopCh:
			return
		case <-ticker.C:
			sl.runAllSources(ctx)
		}
	}
}

// Stop gracefully shuts down the sync loop.
// Safe to call multiple times.
func (sl *SyncLoop) Stop() {
	select {
	case <-sl.stopCh:
		// Already closed, do nothing
	default:
		close(sl.stopCh)
	}
}

// runAllSources fetches all enabled sources and syncs each one.
func (sl *SyncLoop) runAllSources(ctx context.Context) {
	sources, err := sl.storage.ListItemSources(ctx)
	if err != nil {
		log.ErrorLog.Printf("[SyncLoop] ListItemSources error: %v", err)
		return
	}

	for i := range sources {
		src := &sources[i]
		if !src.Enabled {
			continue
		}
		// We need the raw ent.ItemSource for ent field access; call through the ent repo.
		er, ok := sl.storage.repo.(*EntRepository)
		if !ok {
			continue
		}
		entSrc, entErr := er.GetItemSourceByID(ctx, src.ID)
		if entErr != nil {
			log.ErrorLog.Printf("[SyncLoop] GetItemSourceByID(%s) error: %v", src.ID, entErr)
			continue
		}
		if syncErr := sl.SyncOne(ctx, entSrc); syncErr != nil {
			log.ErrorLog.Printf("[SyncLoop] SyncOne(%s plugin=%s) error: %v", src.ID, src.PluginID, syncErr)
		}
	}
}

// decryptConfigToken decrypts an encrypted token in config JSON if needed.
// If the config has "encrypted":true, it decrypts the token field using the provided key function.
// If decryption is not available or not needed, returns the raw config unchanged.
// Exported for testing.
func (sl *SyncLoop) TestDecryptConfigToken(raw string) (string, error) {
	return sl.decryptConfigToken(raw)
}

func (sl *SyncLoop) decryptConfigToken(raw string) (string, error) {
	if raw == "" {
		return raw, nil
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return raw, nil // Not JSON; pass through as-is
	}

	encrypted, _ := cfg["encrypted"].(bool)
	if !encrypted {
		return raw, nil // Not encrypted; pass through
	}

	// Need to decrypt
	if sl.keyFunc == nil {
		return "", fmt.Errorf("config has encrypted token but no key provider available")
	}

	encToken, _ := cfg["token"].(string)
	if encToken == "" {
		return raw, nil // No token to decrypt
	}

	key, err := sl.keyFunc()
	if err != nil {
		return "", fmt.Errorf("get key: %w", err)
	}

	plainToken, err := DecryptToken(key, encToken)
	if err != nil {
		return "", fmt.Errorf("decrypt token: %w", err)
	}

	// Build decrypted config JSON (remove encrypted flag, replace token with plaintext)
	cfg["token"] = plainToken
	delete(cfg, "encrypted")

	decrypted, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("re-encode config: %w", err)
	}

	return string(decrypted), nil
}

// SyncOne fetches and upserts items for a single ItemSource.
func (sl *SyncLoop) SyncOne(ctx context.Context, source *ent.ItemSource) error {
	plugin, ok := sl.registry.Get(source.PluginID)
	if !ok {
		return fmt.Errorf("no plugin registered for plugin_id %q", source.PluginID)
	}

	// Decrypt config if needed before passing to plugin
	decryptedConfig, err := sl.decryptConfigToken(source.Config)
	if err != nil {
		return fmt.Errorf("decrypt config: %w", err)
	}

	cfg := PluginConfig{Raw: decryptedConfig}
	cursor := source.SyncCursor

	items, newCursor, fetchErr := plugin.Fetch(ctx, cfg, cursor)
	if fetchErr != nil {
		return fmt.Errorf("fetch: %w", fetchErr)
	}

	er, ok := sl.storage.repo.(*EntRepository)
	if !ok {
		return fmt.Errorf("SyncOne: storage backend does not support ent operations")
	}

	var created, updated, skipped int

	for _, extItem := range items {
		data := plugin.MapToBacklogItem(extItem, source.ID.String())

		// Check if an item with this external_id already exists.
		existing, lookupErr := er.GetBacklogItemByExternalID(ctx, extItem.ExternalID)
		if lookupErr != nil && !errors.Is(lookupErr, ErrNotFound) {
			log.ErrorLog.Printf("[SyncLoop] GetBacklogItemByExternalID(%s) error: %v", extItem.ExternalID, lookupErr)
			continue
		}

		if errors.Is(lookupErr, ErrNotFound) || existing == nil {
			// New item — create it.
			if _, createErr := sl.storage.CreateBacklogItem(ctx, data); createErr != nil {
				log.ErrorLog.Printf("[SyncLoop] CreateBacklogItem external_id=%s error: %v", extItem.ExternalID, createErr)
				continue
			}
			created++
			continue
		}

		// Existing item — apply local-wins: only update fields not in UserModifiedFields.
		modifiedFields := parseUserModifiedFields(existing.UserModifiedFields)

		update := BacklogItemUpdate{}
		anyField := false

		if !containsField(modifiedFields, "title") {
			update.Title = &data.Title
			anyField = true
		}
		if !containsField(modifiedFields, "description") {
			update.Description = &data.Description
			anyField = true
		}
		if !containsField(modifiedFields, "priority") {
			update.Priority = &data.Priority
			anyField = true
		}
		// Status is always local-wins once user_modified_status_at is set.
		// Status transitions are only done via TransitionBacklogItemStatus — no update here.

		if !anyField {
			skipped++
			continue
		}

		if _, updateErr := sl.storage.UpdateBacklogItem(ctx, existing.ID.String(), update, nil); updateErr != nil {
			log.ErrorLog.Printf("[SyncLoop] UpdateBacklogItem %s error: %v", existing.ID, updateErr)
			continue
		}
		updated++
	}

	// Update cursor and last_synced_at on the source.
	now := time.Now()
	if updateErr := er.UpdateItemSourceSync(ctx, source.ID.String(), newCursor, now); updateErr != nil {
		log.ErrorLog.Printf("[SyncLoop] UpdateItemSourceSync(%s) error: %v", source.ID, updateErr)
	}

	// Record a SourceSyncEvent.
	if syncEventErr := er.CreateSourceSyncEvent(ctx, source.ID.String(), newCursor, created, updated, skipped, now); syncEventErr != nil {
		log.ErrorLog.Printf("[SyncLoop] CreateSourceSyncEvent(%s) error: %v", source.ID, syncEventErr)
	}

	log.InfoLog.Printf("[SyncLoop] source=%s plugin=%s created=%d updated=%d skipped=%d",
		source.ID, source.PluginID, created, updated, skipped)
	return nil
}

// parseUserModifiedFields deserializes UserModifiedFields JSON (e.g. ["title","description"]).
func parseUserModifiedFields(raw string) []string {
	if raw == "" {
		return nil
	}
	var fields []string
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return nil
	}
	return fields
}

// containsField returns true if name is in the fields slice.
func containsField(fields []string, name string) bool {
	for _, f := range fields {
		if f == name {
			return true
		}
	}
	return false
}
