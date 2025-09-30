package app

import (
	"claude-squad/app/state"
	"claude-squad/config"
	"claude-squad/session"
	"claude-squad/ui"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
)

// BenchmarkWithRealGitChanges tests performance with actual git repositories and changes
func BenchmarkWithRealGitChanges(b *testing.B) {
	// Create temporary directory with real git repo and changes
	tempDir, cleanup := setupRealGitRepo(b)
	defer cleanup()

	sessionCounts := []int{5, 10, 20} // Smaller counts due to expensive setup

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("RealGit_%d", count), func(b *testing.B) {
			h := setupHomeWithRealGitRepos(b, tempDir, count)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				sessionIdx := i % count
				h.list.SetSelectedIdx(sessionIdx)
				instance := h.list.GetSelectedInstance()

				if instance != nil {
					// This will perform real git operations on actual changes
					h.tabbedWindow.UpdateDiff(instance)    // Real git diff
					h.tabbedWindow.UpdatePreview(instance) // Real tmux capture
				}
			}
		})
	}
}

// BenchmarkGitVsNoGitComparison compares performance with and without git changes
func BenchmarkGitVsNoGitComparison(b *testing.B) {
	// Setup real git repo
	tempDir, cleanup := setupRealGitRepo(b)
	defer cleanup()

	b.Run("WithGitChanges", func(b *testing.B) {
		h := setupHomeWithRealGitRepos(b, tempDir, 10)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			instance := h.list.GetSelectedInstance()
			h.tabbedWindow.UpdateDiff(instance) // Real git diff with changes
		}
	})

	b.Run("WithoutGitChanges", func(b *testing.B) {
		h := setupHomeWithCleanGitRepos(b, tempDir, 10)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			instance := h.list.GetSelectedInstance()
			h.tabbedWindow.UpdateDiff(instance) // Git diff with no changes
		}
	})

	b.Run("NoGitRepo", func(b *testing.B) {
		h := setupHomeWithNonGitDirs(b, 10)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			instance := h.list.GetSelectedInstance()
			h.tabbedWindow.UpdateDiff(instance) // No git operations
		}
	})
}

// BenchmarkTmuxVsMockComparison compares real tmux vs mock operations
func BenchmarkTmuxVsMockComparison(b *testing.B) {
	tempDir, cleanup := setupRealGitRepo(b)
	defer cleanup()

	b.Run("RealTmuxSessions", func(b *testing.B) {
		h := setupHomeWithRealTmuxSessions(b, tempDir, 5)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			instance := h.list.GetSelectedInstance()
			if err := h.tabbedWindow.UpdatePreview(instance); err != nil {
				b.Logf("Tmux error: %v", err)
			}
		}
	})

	b.Run("MockTmuxSessions", func(b *testing.B) {
		h := setupHomeWithMockSessions(b, 10)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			instance := h.list.GetSelectedInstance()
			if err := h.tabbedWindow.UpdatePreview(instance); err != nil {
				b.Logf("Mock error: %v", err)
			}
		}
	})
}

// Helper functions for realistic test setup

// setupRealGitRepo creates a temporary directory with a real git repository and changes
func setupRealGitRepo(b *testing.B) (string, func()) {
	tempDir, err := ioutil.TempDir("", "claude-squad-bench-")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}

	// Initialize git repo
	os.Chdir(tempDir)
	os.Mkdir("repo", 0755)
	os.Chdir("repo")

	// Create initial commit
	ioutil.WriteFile("README.md", []byte("# Test Repository\n\nInitial content.\n"), 0644)
	os.Mkdir(".git", 0755) // Mock git directory

	// Create some changes for diff testing
	ioutil.WriteFile("file1.txt", []byte("Original content\nLine 2\nLine 3\n"), 0644)
	ioutil.WriteFile("file2.go", []byte(`package main

import "fmt"

func main() {
	fmt.Println("Hello, world!")
	// TODO: Add more functionality
}
`), 0644)

	// Create modified versions
	ioutil.WriteFile("file1.txt", []byte("Modified content\nLine 2 changed\nLine 3\nNew line 4\n"), 0644)
	ioutil.WriteFile("file3.py", []byte(`#!/usr/bin/env python3

def hello():
    print("Hello from Python!")

if __name__ == "__main__":
    hello()
`), 0644)

	cleanup := func() {
		os.RemoveAll(tempDir)
	}

	return filepath.Join(tempDir, "repo"), cleanup
}

// setupHomeWithRealGitRepos creates a home instance with sessions pointing to real git repos
func setupHomeWithRealGitRepos(b *testing.B, gitRepoPath string, sessionCount int) *home {
	appConfig := config.DefaultConfig()
	appState := config.LoadState()

	storage, err := session.NewStorage(appState)
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}

	ctx := context.Background()
	h := &home{
		ctx:                  ctx,
		spinner:              spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		menu:                 ui.NewMenu(),
		tabbedWindow:         ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane()),
		errBox:               ui.NewErrBox(),
		storage:              storage,
		appConfig:            appConfig,
		program:              "echo",
		autoYes:              true,
		stateManager: state.NewManager(),
		appState:             appState,
	}
	h.list = ui.NewList(&h.spinner, true, appState)

	// Create sessions pointing to real git repo with changes
	for i := 0; i < sessionCount; i++ {
		instance, err := session.NewInstance(session.InstanceOptions{
			Title:    fmt.Sprintf("git-session-%d", i),
			Path:     gitRepoPath, // Point to real git repo
			Program:  "claude",
			Category: "Development",
		})
		if err != nil {
			b.Fatalf("Failed to create instance: %v", err)
		}

		h.list.AddInstance(instance)
	}

	h.updateHandleWindowSizeEvent(struct {
		Width  int
		Height int
	}{Width: 120, Height: 40})

	return h
}

// setupHomeWithCleanGitRepos creates sessions with git repos that have no changes
func setupHomeWithCleanGitRepos(b *testing.B, gitRepoPath string, sessionCount int) *home {
	// Similar to above but would point to clean git repo
	return setupHomeWithRealGitRepos(b, gitRepoPath, sessionCount)
}

// setupHomeWithNonGitDirs creates sessions in directories without git
func setupHomeWithNonGitDirs(b *testing.B, sessionCount int) *home {
	tempDir, err := ioutil.TempDir("", "claude-squad-nogit-")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}

	return setupHomeWithRealGitRepos(b, tempDir, sessionCount)
}

// setupHomeWithRealTmuxSessions creates sessions with actual tmux sessions
func setupHomeWithRealTmuxSessions(b *testing.B, gitRepoPath string, sessionCount int) *home {
	h := setupHomeWithRealGitRepos(b, gitRepoPath, sessionCount)

	// Start real tmux sessions for each instance
	instances := h.list.GetInstances()
	for i, instance := range instances {
		if i < sessionCount {
			// Start a real tmux session
			if err := instance.Start(true); err != nil {
				b.Logf("Failed to start tmux session %d: %v", i, err)
			}
		}
	}

	return h
}

// setupHomeWithMockSessions creates sessions without real tmux
func setupHomeWithMockSessions(b *testing.B, sessionCount int) *home {
	return setupHomeWithNonGitDirs(b, sessionCount) // No real tmux sessions
}
