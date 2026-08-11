package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewClaudeConfigManager(t *testing.T) {
	mgr, err := NewClaudeConfigManager()
	if err != nil {
		t.Fatalf("NewClaudeConfigManager() error = %v", err)
	}

	if mgr == nil {
		t.Fatal("NewClaudeConfigManager() returned nil")
	}

	if mgr.claudeDir == "" {
		t.Error("ClaudeConfigManager.claudeDir is empty")
	}
}

func TestGetClaudeDir(t *testing.T) {
	claudeDir, err := GetClaudeDir()
	if err != nil {
		t.Fatalf("GetClaudeDir() error = %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error = %v", err)
	}

	expected := filepath.Join(home, ".claude")
	if claudeDir != expected {
		t.Errorf("GetClaudeDir() = %v, want %v", claudeDir, expected)
	}
}

func TestGetConfig(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create a test config file
	testFile := filepath.Join(tmpDir, "test.md")
	testContent := "# Test Config\nThis is a test"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create manager with test directory
	mgr := &ClaudeConfigManager{
		claudeDir: tmpDir,
	}

	// Test reading existing file
	config, err := mgr.GetConfig("test.md")
	if err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}

	if config.Name != "test.md" {
		t.Errorf("ConfigFile.Name = %v, want test.md", config.Name)
	}

	if config.Content != testContent {
		t.Errorf("ConfigFile.Content = %v, want %v", config.Content, testContent)
	}

	// Test reading non-existent file
	_, err = mgr.GetConfig("nonexistent.md")
	if err == nil {
		t.Error("GetConfig() with non-existent file should return error")
	}
}

func TestListConfigs(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create test config files
	files := map[string]string{
		"CLAUDE.md":     "# Claude Config",
		"settings.json": `{"key": "value"}`,
		"agents.md":     "# Agents",
	}

	for name, content := range files {
		path := filepath.Join(tmpDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", name, err)
		}
	}

	// Create a hidden file (should be skipped)
	hiddenFile := filepath.Join(tmpDir, ".hidden")
	if err := os.WriteFile(hiddenFile, []byte("hidden"), 0644); err != nil {
		t.Fatalf("Failed to create hidden file: %v", err)
	}

	// Create manager with test directory
	mgr := &ClaudeConfigManager{
		claudeDir: tmpDir,
	}

	// Test listing configs
	configs, err := mgr.ListConfigs()
	if err != nil {
		t.Fatalf("ListConfigs() error = %v", err)
	}

	if len(configs) != 3 {
		t.Errorf("ListConfigs() returned %d files, want 3", len(configs))
	}

	// Verify each file is present
	found := make(map[string]bool)
	for _, config := range configs {
		found[config.Name] = true
		expectedContent, ok := files[config.Name]
		if !ok {
			t.Errorf("Unexpected file in ListConfigs(): %s", config.Name)
			continue
		}
		if config.Content != expectedContent {
			t.Errorf("ConfigFile[%s].Content = %v, want %v", config.Name, config.Content, expectedContent)
		}
	}

	for name := range files {
		if !found[name] {
			t.Errorf("Expected file not found in ListConfigs(): %s", name)
		}
	}
}

func TestValidateJSON(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := &ClaudeConfigManager{
		claudeDir: tmpDir,
	}

	tests := []struct {
		name     string
		filename string
		content  string
		wantErr  bool
	}{
		{
			name:     "valid settings.json",
			filename: "settings.json",
			content:  `{"model": "claude-3", "api_key": "test-key"}`,
			wantErr:  false,
		},
		{
			name:     "invalid JSON syntax",
			filename: "settings.json",
			content:  `{"model": "claude-3", "api_key": }`,
			wantErr:  true,
		},
		{
			name:     "non-JSON file passes validation",
			filename: "CLAUDE.md",
			content:  "# This is markdown\nNot JSON at all",
			wantErr:  false,
		},
		{
			name:     "empty JSON object is valid",
			filename: "settings.json",
			content:  `{}`,
			wantErr:  false,
		},
		{
			name:     "JSON with extra fields is valid",
			filename: "settings.json",
			content:  `{"model": "claude-3", "extra_field": "allowed"}`,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mgr.ValidateJSON(tt.filename, tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestUpdateConfigWithValidation(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := &ClaudeConfigManager{
		claudeDir: tmpDir,
	}

	// Test valid JSON update
	validContent := `{"model": "claude-3", "api_key": "test"}`
	err := mgr.UpdateConfigWithValidation("settings.json", validContent)
	if err != nil {
		t.Fatalf("UpdateConfigWithValidation() with valid JSON failed: %v", err)
	}

	// Verify file was written
	config, err := mgr.GetConfig("settings.json")
	if err != nil {
		t.Fatalf("GetConfig() failed: %v", err)
	}
	if config.Content != validContent {
		t.Errorf("File content = %v, want %v", config.Content, validContent)
	}

	// Test invalid JSON update
	invalidContent := `{"invalid": json}`
	err = mgr.UpdateConfigWithValidation("settings.json", invalidContent)
	if err == nil {
		t.Error("UpdateConfigWithValidation() with invalid JSON should fail")
	}

	// Test non-JSON file update (should skip validation)
	mdContent := "# Claude Config\nThis is markdown"
	err = mgr.UpdateConfigWithValidation("CLAUDE.md", mdContent)
	if err != nil {
		t.Fatalf("UpdateConfigWithValidation() with markdown failed: %v", err)
	}
}
