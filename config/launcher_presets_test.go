package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/config"
)

func writeLauncherPresetsFile(t *testing.T, dir, contents string) string {
	t.Helper()
	path := filepath.Join(dir, "launcher-presets.json")
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}
	return path
}

func TestLoadLauncherPresets_should_ReturnPresetsInFileOrder_When_FileIsValid(t *testing.T) {
	dir := t.TempDir()
	path := writeLauncherPresetsFile(t, dir, `{
		"version": 1,
		"presets": [
			{"id": "codex-gpt5", "label": "Codex GPT-5", "argv": ["codex", "--model", "gpt-5"]},
			{"id": "remote-claude", "label": "Remote Claude", "argv": ["ssh", "-t", "host", "cd ~/repo && exec claude"]}
		]
	}`)

	file, err := config.LoadLauncherPresets(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(file.Presets) != 2 {
		t.Fatalf("expected 2 presets, got %d", len(file.Presets))
	}
	if file.Presets[0].ID != "codex-gpt5" || file.Presets[1].ID != "remote-claude" {
		t.Errorf("presets not in file order: %+v", file.Presets)
	}
}

func TestLoadLauncherPresets_should_ReturnParseError_When_JSONHasTrailingComma(t *testing.T) {
	dir := t.TempDir()
	path := writeLauncherPresetsFile(t, dir, `{
		"version": 1,
		"presets": [
			{"id": "codex", "label": "Codex", "argv": ["codex"]},
		]
	}`)

	file, err := config.LoadLauncherPresets(path)
	if err == nil {
		t.Fatal("expected a JSON parse error, got nil")
	}
	if file != nil {
		t.Errorf("expected nil file on parse error, got %+v", file)
	}
}

func TestLoadLauncherPresets_should_ReturnDuplicateIDError_When_TwoPresetsShareID(t *testing.T) {
	dir := t.TempDir()
	path := writeLauncherPresetsFile(t, dir, `{
		"version": 1,
		"presets": [
			{"id": "codex", "label": "Codex A", "argv": ["codex"]},
			{"id": "codex", "label": "Codex B", "argv": ["codex", "--other"]}
		]
	}`)

	file, err := config.LoadLauncherPresets(path)
	if err == nil {
		t.Fatal("expected a duplicate-id error, got nil")
	}
	if file != nil {
		t.Errorf("expected nil file on validation error, got %+v", file)
	}
	msg := err.Error()
	if !strings.Contains(msg, "codex") || !strings.Contains(msg, "1") || !strings.Contains(msg, "2") {
		t.Errorf("expected error to name id %q and both positions, got: %s", "codex", msg)
	}
}

func TestLoadLauncherPresets_should_ReturnEmptyArgvError_When_PresetArgvIsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := writeLauncherPresetsFile(t, dir, `{
		"version": 1,
		"presets": [
			{"id": "codex", "label": "Codex", "argv": []}
		]
	}`)

	file, err := config.LoadLauncherPresets(path)
	if err == nil {
		t.Fatal("expected an empty-argv error, got nil")
	}
	if file != nil {
		t.Errorf("expected nil file on validation error, got %+v", file)
	}
	if !strings.Contains(err.Error(), "codex") {
		t.Errorf("expected error to name preset id %q, got: %s", "codex", err.Error())
	}
}

func TestLoadLauncherPresets_should_ReturnEmptyArgvElementError_When_ArgvContainsBlankString(t *testing.T) {
	dir := t.TempDir()
	path := writeLauncherPresetsFile(t, dir, `{
		"version": 1,
		"presets": [
			{"id": "codex", "label": "Codex", "argv": ["codex", "  "]}
		]
	}`)

	file, err := config.LoadLauncherPresets(path)
	if err == nil {
		t.Fatal("expected an empty-argv-element error, got nil")
	}
	if file != nil {
		t.Errorf("expected nil file on validation error, got %+v", file)
	}
	if !strings.Contains(err.Error(), "codex") {
		t.Errorf("expected error to name preset id %q, got: %s", "codex", err.Error())
	}
}

func TestLoadLauncherPresets_should_ReturnUnsupportedVersionError_When_VersionIsNot1(t *testing.T) {
	dir := t.TempDir()
	path := writeLauncherPresetsFile(t, dir, `{
		"version": 2,
		"presets": [
			{"id": "codex", "label": "Codex", "argv": ["codex"]}
		]
	}`)

	file, err := config.LoadLauncherPresets(path)
	if err == nil {
		t.Fatal("expected an unsupported-version error, got nil")
	}
	if file != nil {
		t.Errorf("expected nil file on validation error, got %+v", file)
	}
	if !strings.Contains(err.Error(), "2") {
		t.Errorf("expected error to name the unsupported version, got: %s", err.Error())
	}
}

func TestLoadLauncherPresets_should_ReturnNotExistError_When_FileIsMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "launcher-presets.json")

	file, err := config.LoadLauncherPresets(path)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected os.IsNotExist(err) to be true, got: %v", err)
	}
	if file != nil {
		t.Errorf("expected nil file, got %+v", file)
	}
}
