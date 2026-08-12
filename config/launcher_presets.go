package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LauncherPreset is one hand-authored entry in launcher-presets.json: a named,
// argv-based launch shortcut (e.g. a specific agent + flags, or a remote-exec
// ssh command). argv is never shell-split — argv[0] maps to Program, argv[1:]
// maps to session.Instance.ExtraArgs, both shell-quoted independently at launch
// time (see buildLaunchCommand in session/instance_tmux.go).
type LauncherPreset struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Argv        []string `json:"argv"`
	Program     string   `json:"program,omitempty"`
	DefaultPath string   `json:"default_path,omitempty"`
}

// LauncherPresetsFile is the top-level document shape of launcher-presets.json.
type LauncherPresetsFile struct {
	Version int              `json:"version"`
	Presets []LauncherPreset `json:"presets"`
}

// DefaultLauncherPresetsPath returns the resolved path to launcher-presets.json,
// honoring the same instance-isolation rules as the rest of config/ (GetConfigDir).
func DefaultLauncherPresetsPath() (string, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "launcher-presets.json"), nil
}

// LoadLauncherPresets reads and validates launcher-presets.json at path.
//
// A missing file is reported via an os.IsNotExist-satisfying error, distinguishable
// from a validation failure — callers treat "not exist" as "zero presets, no error to
// surface" and any other error as a loud, whole-file rejection to surface as load_error.
func LoadLauncherPresets(path string) (*LauncherPresetsFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var file LauncherPresetsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("failed to parse launcher presets: %w", err)
	}

	if err := validateLauncherPresets(&file); err != nil {
		return nil, err
	}

	return &file, nil
}

func validateLauncherPresets(file *LauncherPresetsFile) error {
	if file.Version != 1 {
		return fmt.Errorf("unsupported launcher-presets version %d (expected 1)", file.Version)
	}

	seen := make(map[string]int, len(file.Presets))
	for i, p := range file.Presets {
		if p.ID == "" {
			return fmt.Errorf("preset at position %d has no id", i+1)
		}
		if firstIdx, dup := seen[p.ID]; dup {
			return fmt.Errorf("duplicate preset id %q (positions %d and %d)", p.ID, firstIdx+1, i+1)
		}
		seen[p.ID] = i

		if len(p.Argv) == 0 {
			return fmt.Errorf("preset %q has no argv", p.ID)
		}
		for j, tok := range p.Argv {
			if strings.TrimSpace(tok) == "" {
				return fmt.Errorf("preset %q has an empty argv element at position %d", p.ID, j+1)
			}
		}
		if p.Label == "" {
			return fmt.Errorf("preset %q has no label", p.ID)
		}
	}

	return nil
}
