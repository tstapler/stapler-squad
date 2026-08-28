package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const workspaceMetaFileName = "workspace_meta.json"

// WorkspaceMeta stores display information about a workspace/database.
// Written to each workspace directory at startup to enable workspace discovery.
type WorkspaceMeta struct {
	WorkspaceID string    `json:"workspace_id"` // dir name (hash or instance name)
	Type        string    `json:"type"`         // "workspace", "instance", "shared"
	CWD         string    `json:"cwd"`
	Name        string    `json:"name"`       // last path component of CWD, or "Default"
	ConfigDir   string    `json:"config_dir"` // absolute path to this workspace dir
	LastUsed    time.Time `json:"last_used"`
}

// writeWorkspaceMeta writes metadata for a workspace to disk.
// Uses atomic temp-file + rename to avoid partial writes.
// Errors are silently ignored — this is best-effort metadata.
func writeWorkspaceMeta(configDir, cwd, wsType string) {
	name := filepath.Base(cwd)
	if wsType == "shared" || cwd == "" {
		name = "Default"
	}
	workspaceID := filepath.Base(configDir)

	meta := WorkspaceMeta{
		WorkspaceID: workspaceID,
		Type:        wsType,
		CWD:         cwd,
		Name:        name,
		ConfigDir:   configDir,
		LastUsed:    time.Now(),
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return
	}

	// Atomic write via unique temp file + rename (not a fixed ".tmp" suffix): two
	// concurrent writers targeting the same configDir could otherwise interleave
	// writes and rename a torn file into place. Mirrors config.go's saveConfigLocked
	// fix for the identical hazard.
	tmpFile, err := os.CreateTemp(configDir, workspaceMetaFileName+".*.tmp")
	if err != nil {
		return
	}
	tmpPath := tmpFile.Name()
	_, writeErr := tmpFile.Write(data)
	closeErr := tmpFile.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(tmpPath)
		return
	}
	if err := os.Chmod(tmpPath, 0644); err != nil {
		_ = os.Remove(tmpPath)
		return
	}
	_ = os.Rename(tmpPath, filepath.Join(configDir, workspaceMetaFileName))
}

// ReadWorkspaceMeta reads workspace metadata from the given config directory.
func ReadWorkspaceMeta(configDir string) (WorkspaceMeta, error) {
	data, err := os.ReadFile(filepath.Join(configDir, workspaceMetaFileName))
	if err != nil {
		return WorkspaceMeta{}, fmt.Errorf("failed to read workspace meta: %w", err)
	}
	var meta WorkspaceMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return WorkspaceMeta{}, fmt.Errorf("failed to parse workspace meta: %w", err)
	}
	return meta, nil
}

// GetPreferredWorkspaceFile returns the path to the preferred workspace preference file.
func GetPreferredWorkspaceFile(baseDir string) string {
	return filepath.Join(baseDir, "preferred_workspace")
}

// SetPreferredWorkspace atomically writes the preferred workspace config dir path.
// Pass configDir="" to clear the preference.
func SetPreferredWorkspace(baseDir, configDir string) error {
	prefFile := GetPreferredWorkspaceFile(baseDir)
	if configDir == "" {
		return os.Remove(prefFile)
	}
	// Unique temp name (not prefFile+".tmp") for the same reason writeWorkspaceMeta
	// above uses one — avoids two concurrent SetPreferredWorkspace callers clobbering
	// each other's in-progress write.
	tmpFile, err := os.CreateTemp(baseDir, filepath.Base(prefFile)+".*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp preferred workspace file: %w", err)
	}
	tmpPath := tmpFile.Name()
	_, writeErr := tmpFile.Write([]byte(configDir))
	closeErr := tmpFile.Close()
	if writeErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to write preferred workspace: %w", writeErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to write preferred workspace: %w", closeErr)
	}
	if err := os.Chmod(tmpPath, 0644); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to chmod temp preferred workspace file: %w", err)
	}
	if err := os.Rename(tmpPath, prefFile); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to rename preferred workspace: %w", err)
	}
	return nil
}

// ListAvailableWorkspaces discovers all known workspaces by scanning workspace and instance subdirs.
// Skips test directories. Returns an empty slice (not an error) if none are found.
func ListAvailableWorkspaces(baseDir string) ([]WorkspaceMeta, error) {
	var workspaces []WorkspaceMeta

	// Scan workspaces/ subdirectory
	workspaces = append(workspaces, scanMetaSubdirs(filepath.Join(baseDir, "workspaces"))...)

	// Scan instances/ subdirectory
	workspaces = append(workspaces, scanMetaSubdirs(filepath.Join(baseDir, "instances"))...)

	// Check shared/global state at baseDir itself
	if meta, err := ReadWorkspaceMeta(baseDir); err == nil {
		workspaces = append(workspaces, meta)
	}

	return workspaces, nil
}

// scanMetaSubdirs reads all immediate subdirectories of dir and returns WorkspaceMeta
// for those that contain a metadata file. Skips directories named "test".
func scanMetaSubdirs(dir string) []WorkspaceMeta {
	var metas []WorkspaceMeta
	entries, err := os.ReadDir(dir)
	if err != nil {
		return metas
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Skip test isolation directories
		if entry.Name() == "test" {
			continue
		}
		meta, err := ReadWorkspaceMeta(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		metas = append(metas, meta)
	}
	return metas
}

// EnsureWorkspaceMeta writes workspace metadata for the current configuration directory.
// Should be called once at server startup. Skips test mode directories.
func EnsureWorkspaceMeta() {
	// Don't write meta for test mode (isolated per-PID directories)
	if IsTestMode() {
		return
	}

	configDir, err := GetConfigDir()
	if err != nil {
		return
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}
	baseDir := filepath.Join(homeDir, ".stapler-squad")

	// Determine workspace type and CWD based on environment
	wsType := "workspace"
	cwd := ""

	instanceID := os.Getenv("STAPLER_SQUAD_INSTANCE")
	if instanceID == "shared" || configDir == baseDir {
		wsType = "shared"
	} else if instanceID != "" {
		wsType = "instance"
	} else {
		workDir, err := os.Getwd()
		if err == nil {
			cwd = workDir
		}
	}

	writeWorkspaceMeta(configDir, cwd, wsType)
}
