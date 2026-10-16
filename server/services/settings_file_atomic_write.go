package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/tstapler/stapler-squad/log"
)

// settingsFileLocks serializes the read-merge-write sequence each of InjectHookConfig,
// InjectHooksConfig, and RemoveHooksConfig performs against a given settings.local.json,
// keyed by absolute path. writeSettingsAtomic's unique-tmp-filename write only prevents
// two concurrent writers from tearing/corrupting the file on rename; it does nothing to
// stop a classic lost update, where both callers read the same pre-image, merge
// independently, and the second writer's rename silently clobbers the first caller's
// change. Confirmed real overlapping callers on the same rootDir's settings.local.json:
// backlog_service_triage.go's sync InjectHooksConfig/RemoveHooksConfig during session
// spawn, and session_service.go's async InjectHookConfig in that same spawn flow's
// trackCleanup callback. An in-process sync.Mutex is sufficient — no cross-process
// locking is needed, since all callers run within one stapler-squad server process.
var settingsFileLocks sync.Map // map[string]*sync.Mutex

// lockSettingsPath acquires (creating if needed) the mutex for path and returns a
// function that unlocks it — call via `defer lockSettingsPath(settingsPath)()`.
func lockSettingsPath(path string) func() {
	v, _ := settingsFileLocks.LoadOrStore(path, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func writeSettingsAtomic(settingsPath, claudeDir string, raw map[string]json.RawMessage) error {
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	if err := os.MkdirAll(claudeDir, 0o750); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	// Unique temp file (not settingsPath+".tmp") so two concurrent writers targeting
	// the same settingsPath — e.g. InjectHooksConfig and RemoveHooksConfig racing on
	// the same rootDir from two goroutines — can't clobber each other's temp file
	// mid-write and produce a truncated/corrupt settingsPath after rename. Mirrors
	// internal/claudehooks/claudehooks.go's mutate(), which documents the same fix
	// for the identical fixed-tmp-filename hazard.
	tmp, err := os.CreateTemp(claudeDir, filepath.Base(settingsPath)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", claudeDir, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) //nolint:errcheck // best-effort cleanup if rename fails
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp %s: %w", tmpPath, err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("chmod temp %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, settingsPath); err != nil {
		return fmt.Errorf("rename %s: %w", tmpPath, err)
	}
	// Shared by InjectHooksConfig, RemoveHooksConfig, and InjectHookConfig — no single
	// caller-specific tag applies, so this stays generic.
	log.Info("[writeSettingsAtomic] wrote settings", "path", settingsPath)
	return nil
}
