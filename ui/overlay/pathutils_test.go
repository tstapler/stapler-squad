package overlay

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidatePathEnhanced(t *testing.T) {
	tempDir := t.TempDir()

	// Create test directory structure
	validDir := filepath.Join(tempDir, "valid-dir")
	readOnlyDir := filepath.Join(tempDir, "readonly-dir")

	if err := os.MkdirAll(validDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(readOnlyDir, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(readOnlyDir, 0755) // Restore permissions for cleanup

	// Create a test file (not directory)
	testFile := filepath.Join(tempDir, "test-file.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name                 string
		path                 string
		expectValid          bool
		expectErrorContains  string
		expectWarnings       []string
		expectIsGitRepo      bool
	}{
		{
			name:        "valid directory",
			path:        validDir,
			expectValid: true,
		},
		{
			name:                "empty path",
			path:                "",
			expectValid:         false,
			expectErrorContains: "Path cannot be empty",
		},
		{
			name:                "whitespace only path",
			path:                "   ",
			expectValid:         false,
			expectErrorContains: "Path cannot be empty",
		},
		{
			name:                "non-existent path",
			path:                filepath.Join(tempDir, "nonexistent"),
			expectValid:         false,
			expectErrorContains: "does not exist",
		},
		{
			name:                "file instead of directory",
			path:                testFile,
			expectValid:         false,
			expectErrorContains: "not a directory",
		},
		{
			name:                "read-only directory",
			path:                readOnlyDir,
			expectValid:         true,
			expectWarnings:      []string{"not writable"},
		},
		{
			name:        "current directory",
			path:        ".",
			expectValid: true,
		},
		{
			name:        "home directory tilde",
			path:        "~",
			expectValid: true,
		},
		{
			name:                "invalid characters",
			path:                string([]byte{0x00, 'p', 'a', 't', 'h'}), // null byte
			expectValid:         false,
			expectErrorContains: "invalid characters",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ValidatePathEnhanced(tc.path)

			// Check validity
			if result.Valid != tc.expectValid {
				t.Errorf("Expected Valid=%v, got %v", tc.expectValid, result.Valid)
			}

			// Check error message
			if tc.expectErrorContains != "" {
				if result.ErrorMessage == "" {
					t.Errorf("Expected error message containing '%s', got empty message", tc.expectErrorContains)
				} else if !strings.Contains(strings.ToLower(result.ErrorMessage), strings.ToLower(tc.expectErrorContains)) {
					t.Errorf("Expected error message containing '%s', got '%s'", tc.expectErrorContains, result.ErrorMessage)
				}
			} else if result.ErrorMessage != "" && result.Valid {
				t.Errorf("Expected no error message for valid path, got '%s'", result.ErrorMessage)
			}

			// Check warnings
			for _, expectedWarning := range tc.expectWarnings {
				found := false
				for _, warning := range result.Warnings {
					if strings.Contains(strings.ToLower(warning), strings.ToLower(expectedWarning)) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected warning containing '%s', warnings: %v", expectedWarning, result.Warnings)
				}
			}

			// Check expanded path is set when valid
			if result.Valid && result.ExpandedPath == "" {
				t.Error("Expected ExpandedPath to be set for valid path")
			}

			// For valid paths, check that expanded path exists
			if result.Valid && !PathExists(result.ExpandedPath) {
				t.Errorf("Valid path should have existing ExpandedPath, got: %s", result.ExpandedPath)
			}

			t.Logf("Path: %s, Valid: %v, Error: %s, Warnings: %v",
				tc.path, result.Valid, result.ErrorMessage, result.Warnings)
		})
	}
}

func TestValidatePathQuick(t *testing.T) {
	tempDir := t.TempDir()

	// Create test directory
	validDir := filepath.Join(tempDir, "valid")
	if err := os.MkdirAll(validDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create test file
	testFile := filepath.Join(tempDir, "file.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name        string
		path        string
		expectError bool
	}{
		{
			name:        "valid directory",
			path:        validDir,
			expectError: false,
		},
		{
			name:        "empty path",
			path:        "",
			expectError: true,
		},
		{
			name:        "non-existent path",
			path:        filepath.Join(tempDir, "nonexistent"),
			expectError: true,
		},
		{
			name:        "file not directory",
			path:        testFile,
			expectError: true,
		},
		{
			name:        "current directory",
			path:        ".",
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePathQuick(tc.path)

			if tc.expectError && err == nil {
				t.Error("Expected error but got none")
			} else if !tc.expectError && err != nil {
				t.Errorf("Expected no error but got: %s", err)
			}
		})
	}
}

func TestPathPermissions(t *testing.T) {
	tempDir := t.TempDir()

	// Create test directories with different permissions
	readWriteDir := filepath.Join(tempDir, "rw")
	readOnlyDir := filepath.Join(tempDir, "ro")

	if err := os.MkdirAll(readWriteDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(readOnlyDir, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(readOnlyDir, 0755) // Restore for cleanup

	t.Run("read-write directory", func(t *testing.T) {
		perms := checkPathPermissions(readWriteDir)

		if !perms.Readable {
			t.Error("Expected directory to be readable")
		}
		if !perms.Writable {
			t.Error("Expected directory to be writable")
		}
		if !perms.Executable {
			t.Error("Expected directory to be executable")
		}
	})

	t.Run("read-only directory", func(t *testing.T) {
		perms := checkPathPermissions(readOnlyDir)

		if !perms.Readable {
			t.Error("Expected directory to be readable")
		}
		if perms.Writable {
			t.Error("Expected directory to NOT be writable")
		}
		if !perms.Executable {
			t.Error("Expected directory to be executable")
		}
	})
}

func TestPathCharacterValidation(t *testing.T) {
	testCases := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "normal path",
			path:     "/normal/path",
			expected: false,
		},
		{
			name:     "path with spaces",
			path:     "/path with spaces",
			expected: false,
		},
		{
			name:     "path with null byte",
			path:     string([]byte{'/', 0x00, 'p', 'a', 't', 'h'}),
			expected: true,
		},
		{
			name:     "extremely long path",
			path:     "/" + strings.Repeat("a", 5000),
			expected: true,
		},
		{
			name:     "reasonable long path",
			path:     "/" + strings.Repeat("a", 200),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := containsInvalidChars(tc.path)
			if result != tc.expected {
				t.Errorf("Expected %v for path validation, got %v", tc.expected, result)
			}
		})
	}
}

func TestNetworkPathDetection(t *testing.T) {
	testCases := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "local path",
			path:     "/home/user/project",
			expected: false,
		},
		{
			name:     "UNC path",
			path:     "//server/share",
			expected: true,
		},
		{
			name:     "mount path",
			path:     "/mnt/network-drive",
			expected: true,
		},
		{
			name:     "nfs path",
			path:     "/net/nfs-server/data",
			expected: true,
		},
		{
			name:     "path with nfs in name",
			path:     "/home/nfs-user/project",
			expected: true, // Current implementation is simple
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isNetworkPath(tc.path)
			if result != tc.expected {
				t.Errorf("Expected %v for network path detection, got %v", tc.expected, result)
			}
		})
	}
}

func TestGitRepositoryDetection(t *testing.T) {
	tempDir := t.TempDir()

	// Create a directory with .git subdirectory
	gitRepo := filepath.Join(tempDir, "git-repo")
	gitDir := filepath.Join(gitRepo, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a regular directory
	regularDir := filepath.Join(tempDir, "regular-dir")
	if err := os.MkdirAll(regularDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Run("git repository", func(t *testing.T) {
		if !isGitRepository(gitRepo) {
			t.Error("Expected directory to be detected as Git repository")
		}
	})

	t.Run("regular directory", func(t *testing.T) {
		if isGitRepository(regularDir) {
			t.Error("Expected directory to NOT be detected as Git repository")
		}
	})

	t.Run("non-existent directory", func(t *testing.T) {
		nonExistent := filepath.Join(tempDir, "nonexistent")
		if isGitRepository(nonExistent) {
			t.Error("Expected non-existent directory to NOT be detected as Git repository")
		}
	})
}

func TestDisplayPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot get home directory, skipping test")
	}

	testCases := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "home directory",
			path:     home,
			expected: "~",
		},
		{
			name:     "subdirectory of home",
			path:     filepath.Join(home, "projects"),
			expected: "~/projects",
		},
		{
			name:     "root directory",
			path:     "/tmp",
			expected: "/tmp",
		},
		{
			name:     "relative path",
			path:     "relative/path",
			expected: "relative/path",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getDisplayPath(tc.path)
			if result != tc.expected {
				t.Errorf("Expected display path '%s', got '%s'", tc.expected, result)
			}
		})
	}
}

// BenchmarkValidatePathEnhanced benchmarks the enhanced validation
func BenchmarkValidatePathEnhanced(b *testing.B) {
	tempDir := b.TempDir()
	testDir := filepath.Join(tempDir, "test")
	os.MkdirAll(testDir, 0755)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = ValidatePathEnhanced(testDir)
	}
}

// BenchmarkValidatePathQuick benchmarks the quick validation
func BenchmarkValidatePathQuick(b *testing.B) {
	tempDir := b.TempDir()
	testDir := filepath.Join(tempDir, "test")
	os.MkdirAll(testDir, 0755)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = ValidatePathQuick(testDir)
	}
}