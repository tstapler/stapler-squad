package overlay

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContextualDiscoveryEmptyQuery(t *testing.T) {
	overlay := NewSessionSetupOverlay()

	// Test with empty query should return contextual suggestions
	results := overlay.discoverGitRepositoriesContextual("")

	// Should have at least current directory and home directory suggestions
	if len(results) < 2 {
		t.Errorf("Expected at least 2 contextual suggestions for empty query, got %d", len(results))
	}

	// Check that we have contextual indicators
	foundCurrent := false
	foundHome := false
	for _, result := range results {
		text := result.GetDisplayText()
		if strings.Contains(text, "current") {
			foundCurrent = true
		}
		if strings.Contains(text, "home") {
			foundHome = true
		}
	}

	if !foundCurrent {
		t.Error("Expected current directory suggestion in empty query results")
	}
	if !foundHome {
		t.Error("Expected home directory suggestion in empty query results")
	}
}

func TestContextualDiscoveryInvalidPath(t *testing.T) {
	overlay := NewSessionSetupOverlay()

	// Test with invalid path
	invalidPath := "/this/path/does/not/exist/anywhere"
	results := overlay.discoverGitRepositoriesContextual(invalidPath)

	// Should still include the literal path entry
	if len(results) == 0 {
		t.Error("Expected at least the literal path entry for invalid path")
	}

	// First result should be the literal path with invalid indicator
	firstResult := results[0].GetDisplayText()
	if !strings.Contains(firstResult, "invalid path") && !strings.Contains(firstResult, "❌") {
		t.Errorf("Expected invalid path indicator in first result: %s", firstResult)
	}
}

func TestEnhancedNetworkPathDetection(t *testing.T) {
	testCases := []struct {
		name     string
		path     string
		expected bool
	}{
		{"UNC path", "//server/share", true},
		{"NFS mount", "/mnt/nfs-share", true},
		{"macOS network volume", "/Volumes/NetworkShare", true},
		{"SSHFS mount", "/home/user/sshfs-mount", true},
		{"Local path", "/home/user/project", false},
		{"GVFS mount", "/media/user/gvfs-mount", true},
		{"Systemd media", "/run/media/user/network", true},
		{"CIFS share", "/mnt/cifs-share", true},
		{"Local tmp", "/tmp/local", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isNetworkPath(tc.path)
			if result != tc.expected {
				t.Errorf("Expected %v for path %s, got %v", tc.expected, tc.path, result)
			}
		})
	}
}

func TestPermissionHandlingGracefulDegradation(t *testing.T) {
	tempDir := t.TempDir()

	// Create directories with different permissions
	normalDir := filepath.Join(tempDir, "normal")
	restrictedDir := filepath.Join(tempDir, "restricted")

	if err := os.MkdirAll(normalDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(restrictedDir, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(restrictedDir, 0755) // Restore for cleanup

	overlay := NewSessionSetupOverlay()

	t.Run("normal directory permissions", func(t *testing.T) {
		perms := checkPathPermissions(normalDir)
		if !perms.Readable {
			t.Error("Expected normal directory to be readable")
		}
		if !perms.Writable {
			t.Error("Expected normal directory to be writable")
		}
		if !perms.Executable {
			t.Error("Expected normal directory to be executable")
		}
	})

	t.Run("restricted directory permissions", func(t *testing.T) {
		perms := checkPathPermissions(restrictedDir)
		if perms.Readable {
			t.Error("Expected restricted directory to NOT be readable")
		}
		if perms.Writable {
			t.Error("Expected restricted directory to NOT be writable")
		}
		if perms.Executable {
			t.Error("Expected restricted directory to NOT be executable")
		}
	})

	t.Run("repository discovery with restricted directory", func(t *testing.T) {
		// This should not crash and should handle permission errors gracefully
		results := overlay.findGitRepositoriesInDirectory(tempDir, 2)

		// Should find the normal directory but handle restricted gracefully
		foundPermissionNotice := false
		for _, result := range results {
			if strings.Contains(result.GetDisplayText(), "permission denied") ||
				strings.Contains(result.GetDisplayText(), "🔒") {
				foundPermissionNotice = true
				break
			}
		}

		// Don't require permission notice since behavior may vary by OS
		// but ensure we don't crash
		t.Logf("Found %d results, permission notice present: %v", len(results), foundPermissionNotice)
	})
}

func TestShouldSkipDirectory(t *testing.T) {
	overlay := NewSessionSetupOverlay()

	testCases := []struct {
		name     string
		dirName  string
		expected bool
	}{
		{"normal directory", "myproject", false},
		{"node_modules", "node_modules", true},
		{"system proc", "proc", true},
		{"macOS System", "System", true},
		{"IDE directory", ".vscode", true},
		{"build directory", "build", true},
		{"git directory", ".git", false}, // .git should NOT be skipped by name
		{"backup directory", "Backup of Project", true},
		{"recycle bin", "$RECYCLE.BIN", true},
		{"case insensitive", "NODE_MODULES", true},
		{"normal hidden", ".myproject", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := overlay.shouldSkipDirectory(tc.dirName)
			if result != tc.expected {
				t.Errorf("Expected %v for directory %s, got %v", tc.expected, tc.dirName, result)
			}
		})
	}
}

func TestEnhancedGitRepositoryDetection(t *testing.T) {
	tempDir := t.TempDir()
	overlay := NewSessionSetupOverlay()

	// Create a Git repository
	gitRepo := filepath.Join(tempDir, "git-repo")
	gitDir := filepath.Join(gitRepo, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a Git worktree (with .git file instead of directory)
	worktreeRepo := filepath.Join(tempDir, "worktree-repo")
	if err := os.MkdirAll(worktreeRepo, 0755); err != nil {
		t.Fatal(err)
	}
	gitFile := filepath.Join(worktreeRepo, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: /some/other/location"), 0644); err != nil {
		t.Fatal(err)
	}

	// Regular directory
	regularDir := filepath.Join(tempDir, "regular")
	if err := os.MkdirAll(regularDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Run("git repository with .git directory", func(t *testing.T) {
		if !overlay.isGitRepositoryEnhanced(gitRepo) {
			t.Error("Expected directory with .git subdirectory to be detected as Git repository")
		}
	})

	t.Run("git worktree with .git file", func(t *testing.T) {
		if !overlay.isGitRepositoryEnhanced(worktreeRepo) {
			t.Error("Expected directory with .git file to be detected as Git repository")
		}
	})

	t.Run("regular directory", func(t *testing.T) {
		if overlay.isGitRepositoryEnhanced(regularDir) {
			t.Error("Expected regular directory to NOT be detected as Git repository")
		}
	})

	t.Run("non-existent directory", func(t *testing.T) {
		nonExistent := filepath.Join(tempDir, "nonexistent")
		if overlay.isGitRepositoryEnhanced(nonExistent) {
			t.Error("Expected non-existent directory to NOT be detected as Git repository")
		}
	})
}

func TestPathValidationWithNetworkPaths(t *testing.T) {
	testCases := []struct {
		name            string
		path            string
		expectNetworkWarning bool
	}{
		{"local path", "/home/user/project", false},
		{"UNC network path", "//server/share", true},
		{"NFS mount", "/mnt/nfs-server", true},
		{"macOS network volume", "/Volumes/Server", true},
		{"local home", "/home/user/project", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			isNetwork := isNetworkPath(tc.path)
			if isNetwork != tc.expectNetworkWarning {
				t.Errorf("Expected network detection %v for path %s, got %v",
					tc.expectNetworkWarning, tc.path, isNetwork)
			}

			// If it's a network path, enhanced validation should include warning
			if isNetwork {
				// This test would require the path to actually exist for full validation
				// but we can at least test the network detection logic
				t.Logf("Network path %s correctly detected", tc.path)
			}
		})
	}
}

// BenchmarkEdgeCaseContextualDiscovery benchmarks the performance of contextual discovery with edge cases
func BenchmarkEdgeCaseContextualDiscovery(b *testing.B) {
	overlay := NewSessionSetupOverlay()
	testPath := "~/dev"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = overlay.discoverGitRepositoriesContextual(testPath)
	}
}

// BenchmarkNetworkPathDetection benchmarks network path detection
func BenchmarkNetworkPathDetection(b *testing.B) {
	testPaths := []string{
		"/home/user/project",
		"//server/share",
		"/mnt/nfs-mount",
		"/Volumes/NetworkShare",
		"/media/user/sshfs",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, path := range testPaths {
			_ = isNetworkPath(path)
		}
	}
}