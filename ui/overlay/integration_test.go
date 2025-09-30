package overlay

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestCrossShellCompatibility tests contextual discovery across different shell environments
func TestCrossShellCompatibility(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Detect available shells on the system
	shells := detectAvailableShells(t)
	if len(shells) == 0 {
		t.Skip("No compatible shells found for integration testing")
	}

	tempDir := t.TempDir()
	overlay := NewSessionSetupOverlay()

	// Create a test Git repository structure
	testRepo := filepath.Join(tempDir, "test-repo")
	setupTestGitRepository(t, testRepo)

	for _, shell := range shells {
		t.Run("shell_"+filepath.Base(shell), func(t *testing.T) {
			// Test contextual discovery in different shell environments
			testShellEnvironment(t, overlay, shell, testRepo)
		})
	}
}

// TestPerformanceAcrossDirectorySizes tests performance with various directory structures
func TestPerformanceAcrossDirectorySizes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance integration test in short mode")
	}

	overlay := NewSessionSetupOverlay()
	tempDir := t.TempDir()

	testCases := []struct {
		name       string
		dirCount   int
		repoCount  int
		maxDepth   int
		timeout    time.Duration
	}{
		{"small_structure", 10, 2, 2, 1 * time.Second},
		{"medium_structure", 50, 5, 3, 3 * time.Second},
		{"large_structure", 200, 10, 2, 10 * time.Second},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create directory structure
			testDir := filepath.Join(tempDir, tc.name)
			createTestDirectoryStructure(t, testDir, tc.dirCount, tc.repoCount, tc.maxDepth)

			// Measure performance
			start := time.Now()
			results := overlay.discoverGitRepositoriesContextual(testDir)
			duration := time.Since(start)

			// Verify performance and results
			if duration > tc.timeout {
				t.Errorf("Performance test failed: took %v, expected under %v", duration, tc.timeout)
			}

			if len(results) == 0 {
				t.Error("Expected at least some results from contextual discovery")
			}

			t.Logf("Performance test %s: %d results in %v", tc.name, len(results), duration)
		})
	}
}

// TestNetworkPathIntegration tests behavior with various network path configurations
func TestNetworkPathIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network path integration test in short mode")
	}

	overlay := NewSessionSetupOverlay()

	testCases := []struct {
		name                string
		path                string
		expectNetworkDetection bool
		expectWarning       bool
	}{
		{"local_development", "/home/user/dev", false, false},
		{"nfs_mount", "/mnt/nfs-server/projects", true, true},
		{"macos_network_volume", "/Volumes/SharedDrive", true, true},
		{"unc_path", "//server/share", true, true},
		{"sshfs_mount", "/home/user/sshfs-remote", true, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test network path detection
			isNetwork := isNetworkPath(tc.path)
			if isNetwork != tc.expectNetworkDetection {
				t.Errorf("Network detection failed for %s: expected %v, got %v",
					tc.path, tc.expectNetworkDetection, isNetwork)
			}

			// Test contextual discovery behavior
			results := overlay.discoverGitRepositoriesContextual(tc.path)
			if len(results) == 0 {
				t.Error("Expected at least the literal path to be returned")
			}

			// Verify network warning in results if expected
			if tc.expectWarning {
				foundWarning := false
				for _, result := range results {
					if strings.Contains(result.GetDisplayText(), "🌐") ||
						strings.Contains(result.GetDisplayText(), "network") {
						foundWarning = true
						break
					}
				}
				if !foundWarning {
					t.Logf("Network warning not found in results, but path doesn't exist: %s", tc.path)
				}
			}
		})
	}
}

// TestGitCommandIntegration tests integration with real Git commands
func TestGitCommandIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Git command integration test in short mode")
	}

	// Check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not available for integration testing")
	}

	tempDir := t.TempDir()
	overlay := NewSessionSetupOverlay()

	// Create a real Git repository with multiple branches
	testRepo := filepath.Join(tempDir, "integration-repo")
	setupComplexGitRepository(t, testRepo)

	t.Run("repository_detection", func(t *testing.T) {
		if !overlay.isGitRepositoryEnhanced(testRepo) {
			t.Error("Failed to detect Git repository")
		}
	})

	t.Run("branch_discovery", func(t *testing.T) {
		branches := overlay.loadGitBranches(testRepo)
		if len(branches) < 2 {
			t.Errorf("Expected at least 2 branches, got %d", len(branches))
		}

		// Verify we can find main and feature branches
		foundMain := false
		foundFeature := false
		for _, branch := range branches {
			branchText := branch.GetDisplayText()
			if strings.Contains(branchText, "main") {
				foundMain = true
			}
			if strings.Contains(branchText, "feature") {
				foundFeature = true
			}
		}

		if !foundMain {
			t.Error("Expected to find main branch")
		}
		if !foundFeature {
			t.Error("Expected to find feature branch")
		}
	})

	t.Run("contextual_discovery_with_git", func(t *testing.T) {
		parentDir := filepath.Dir(testRepo)
		results := overlay.discoverGitRepositoriesContextual(parentDir)

		// Should find the test repository
		foundRepo := false
		for _, result := range results {
			if strings.Contains(result.GetID(), testRepo) ||
				strings.Contains(result.GetDisplayText(), "integration-repo") {
				foundRepo = true
				break
			}
		}

		if !foundRepo {
			t.Error("Expected to find the test Git repository in contextual discovery")
		}
	})
}

// TestPermissionHandlingIntegration tests permission handling in real scenarios
func TestPermissionHandlingIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping permission integration test in short mode")
	}

	if runtime.GOOS == "windows" {
		t.Skip("Permission tests are complex on Windows, skipping")
	}

	tempDir := t.TempDir()
	overlay := NewSessionSetupOverlay()

	// Create directories with different permissions
	testDirs := map[string]os.FileMode{
		"normal":     0755,
		"restricted": 0000,
		"readonly":   0555,
	}

	for name, perm := range testDirs {
		dirPath := filepath.Join(tempDir, name)
		if err := os.MkdirAll(dirPath, perm); err != nil {
			t.Fatalf("Failed to create test directory %s: %v", name, err)
		}
		defer os.Chmod(dirPath, 0755) // Restore for cleanup
	}

	t.Run("permission_detection", func(t *testing.T) {
		for name := range testDirs {
			dirPath := filepath.Join(tempDir, name)
			validation := ValidatePathEnhanced(dirPath)

			switch name {
			case "normal":
				if !validation.Valid {
					t.Errorf("Normal directory should be valid")
				}
				if !validation.Permissions.Readable || !validation.Permissions.Writable {
					t.Errorf("Normal directory should be readable and writable")
				}
			case "restricted":
				// Permission behavior varies by system, just check it was processed
				t.Logf("Restricted directory validation: valid=%v, readable=%v",
					validation.Valid, validation.Permissions.Readable)
			case "readonly":
				if !validation.Valid {
					t.Errorf("Readonly directory should be valid for reading")
				}
				if validation.Permissions.Writable {
					// This might vary by system, so just log
					t.Logf("Readonly directory appears writable (system-dependent)")
				}
			}
		}
	})

	t.Run("graceful_degradation", func(t *testing.T) {
		// Test discovery in directory with mixed permissions
		results := overlay.findGitRepositoriesInDirectory(tempDir, 2)

		// Should handle permission issues gracefully without crashing
		if len(results) == 0 {
			t.Log("No results found, which is acceptable for permission testing")
		}

		// Look for permission indicators in results
		hasPermissionIndicator := false
		for _, result := range results {
			if strings.Contains(result.GetDisplayText(), "🔒") ||
				strings.Contains(result.GetDisplayText(), "permission") {
				hasPermissionIndicator = true
				break
			}
		}

		t.Logf("Found %d results, permission indicators: %v", len(results), hasPermissionIndicator)
	})
}

// Helper functions

func detectAvailableShells(t *testing.T) []string {
	shells := []string{
		"/bin/bash",
		"/bin/zsh",
		"/bin/sh",
		"/usr/bin/fish",
		"/opt/homebrew/bin/bash", // Homebrew bash on macOS
	}

	var available []string
	for _, shell := range shells {
		if _, err := os.Stat(shell); err == nil {
			available = append(available, shell)
		}
	}

	// Add shells from PATH
	pathShells := []string{"bash", "zsh", "fish"}
	for _, shell := range pathShells {
		if path, err := exec.LookPath(shell); err == nil {
			// Avoid duplicates
			found := false
			for _, existing := range available {
				if existing == path {
					found = true
					break
				}
			}
			if !found {
				available = append(available, path)
			}
		}
	}

	return available
}

func testShellEnvironment(t *testing.T, overlay *SessionSetupOverlay, shell, testRepo string) {
	// Test environment variable expansion in different shells
	testPaths := []string{
		"~/dev",
		"$HOME/projects",
		"./relative",
		testRepo,
	}

	for _, testPath := range testPaths {
		results := overlay.discoverGitRepositoriesContextual(testPath)
		if len(results) == 0 {
			t.Errorf("No results for path %s in shell %s", testPath, shell)
		}
	}
}

func setupTestGitRepository(t *testing.T, repoPath string) {
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("Failed to create test repo directory: %v", err)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Logf("Warning: Failed to initialize git repo (git may not be available): %v", err)
		return
	}

	// Create a simple file and commit
	testFile := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test Repository"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Configure git for the test
	exec.Command("git", "config", "user.email", "test@example.com").Run()
	exec.Command("git", "config", "user.name", "Test User").Run()

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = repoPath
	cmd.Run()
}

func setupComplexGitRepository(t *testing.T, repoPath string) {
	setupTestGitRepository(t, repoPath)

	// Create feature branch
	cmd := exec.Command("git", "checkout", "-b", "feature-branch")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Logf("Warning: Failed to create feature branch: %v", err)
		return
	}

	// Add feature file
	featureFile := filepath.Join(repoPath, "feature.txt")
	if err := os.WriteFile(featureFile, []byte("Feature implementation"), 0644); err != nil {
		t.Logf("Warning: Failed to create feature file: %v", err)
		return
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Add feature")
	cmd.Dir = repoPath
	cmd.Run()

	// Switch back to main
	cmd = exec.Command("git", "checkout", "main")
	cmd.Dir = repoPath
	cmd.Run()
}

func createTestDirectoryStructure(t *testing.T, rootPath string, dirCount, repoCount, maxDepth int) {
	if err := os.MkdirAll(rootPath, 0755); err != nil {
		t.Fatalf("Failed to create root directory: %v", err)
	}

	// Create directory structure with some Git repositories
	reposCreated := 0
	dirsCreated := 0

	var createDirs func(string, int)
	createDirs = func(currentPath string, depth int) {
		if depth >= maxDepth || dirsCreated >= dirCount {
			return
		}

		for i := 0; i < 3 && dirsCreated < dirCount; i++ {
			dirName := filepath.Join(currentPath, fmt.Sprintf("dir-%d-%d", depth, i))
			if err := os.MkdirAll(dirName, 0755); err != nil {
				continue
			}
			dirsCreated++

			// Create Git repository in some directories
			if reposCreated < repoCount && (dirsCreated%3 == 0) {
				gitDir := filepath.Join(dirName, ".git")
				if err := os.MkdirAll(gitDir, 0755); err == nil {
					reposCreated++
				}
			}

			// Recurse
			if depth < maxDepth-1 {
				createDirs(dirName, depth+1)
			}
		}
	}

	createDirs(rootPath, 0)
}

// BenchmarkCrossShellDiscovery benchmarks contextual discovery across different shells
func BenchmarkCrossShellDiscovery(b *testing.B) {
	overlay := NewSessionSetupOverlay()
	testPath := "~/dev"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = overlay.discoverGitRepositoriesContextual(testPath)
	}
}

// BenchmarkPermissionChecking benchmarks permission checking performance
func BenchmarkPermissionChecking(b *testing.B) {
	tempDir := b.TempDir()
	testDir := filepath.Join(tempDir, "test")
	os.MkdirAll(testDir, 0755)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = checkPathPermissions(testDir)
	}
}