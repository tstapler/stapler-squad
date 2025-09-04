package ui

import (
	"fmt"
	"testing"

	"claude-squad/ui/overlay"
	tea "github.com/charmbracelet/bubbletea"
)

func TestGitStatusOverlay_Snapshots(t *testing.T) {
	renderer := NewTestRenderer().
		SetSnapshotPath("snapshots/git_status/snapshots").
		SetDimensions(80, 25).
		DisableColors()

	tests := []struct {
		name     string
		setup    func() *overlay.GitStatusOverlay
		snapshot string
	}{
		{
			name: "empty_git_status",
			setup: func() *overlay.GitStatusOverlay {
				gitOverlay := overlay.NewGitStatusOverlay()
				gitOverlay.SetSize(80, 25)
				gitOverlay.SetFiles([]overlay.GitFileStatus{}, "main")
				return gitOverlay
			},
			snapshot: "empty.txt",
		},
		{
			name: "mixed_file_states",
			setup: func() *overlay.GitStatusOverlay {
				gitOverlay := overlay.NewGitStatusOverlay()
				gitOverlay.SetSize(80, 25)
				files := []overlay.GitFileStatus{
					{Path: "app/app.go", Staged: true, Modified: false, Untracked: false, StatusChar: "A"},
					{Path: "keys/keys.go", Staged: false, Modified: true, Untracked: false, StatusChar: "M"},
					{Path: "ui/overlay/gitStatusOverlay.go", Staged: false, Modified: false, Untracked: true, StatusChar: "??"},
					{Path: "README.md", Staged: true, Modified: true, Untracked: false, StatusChar: "AM"},
					{Path: "config.toml", Staged: false, Modified: true, Untracked: false, StatusChar: "M"},
				}
				gitOverlay.SetFiles(files, "feature/fugitive-git")
				return gitOverlay
			},
			snapshot: "mixed_files.txt",
		},
		{
			name: "many_files_scrolling",
			setup: func() *overlay.GitStatusOverlay {
				gitOverlay := overlay.NewGitStatusOverlay()
				gitOverlay.SetSize(80, 25)
				var files []overlay.GitFileStatus
				for i := 1; i <= 20; i++ {
					files = append(files, overlay.GitFileStatus{
						Path:       fmt.Sprintf("file_%02d.go", i),
						Staged:     i%3 == 0,
						Modified:   i%2 == 0,
						Untracked:  i%4 == 0,
						StatusChar: "M",
					})
				}
				gitOverlay.SetFiles(files, "develop")
				return gitOverlay
			},
			snapshot: "many_files.txt",
		},
		{
			name: "help_displayed",
			setup: func() *overlay.GitStatusOverlay {
				gitOverlay := overlay.NewGitStatusOverlay()
				gitOverlay.SetSize(80, 30) // Taller to show help
				files := []overlay.GitFileStatus{
					{Path: "src/main.go", Staged: false, Modified: true, StatusChar: "M"},
					{Path: "test/test.go", Staged: true, Modified: false, StatusChar: "A"},
				}
				gitOverlay.SetFiles(files, "main")
				// Help is shown by default, but we can test with it explicitly enabled
				return gitOverlay
			},
			snapshot: "with_help.txt",
		},
		{
			name: "long_branch_name",
			setup: func() *overlay.GitStatusOverlay {
				gitOverlay := overlay.NewGitStatusOverlay()
				gitOverlay.SetSize(80, 25)
				files := []overlay.GitFileStatus{
					{Path: "app.go", Staged: true, StatusChar: "A"},
				}
				gitOverlay.SetFiles(files, "feature/very-long-branch-name-that-might-get-truncated")
				return gitOverlay
			},
			snapshot: "long_branch.txt",
		},
		{
			name: "staged_and_modified_same_file",
			setup: func() *overlay.GitStatusOverlay {
				gitOverlay := overlay.NewGitStatusOverlay()
				gitOverlay.SetSize(80, 25)
				files := []overlay.GitFileStatus{
					{Path: "complex_file.go", Staged: true, Modified: true, StatusChar: "AM"},
					{Path: "staged_only.go", Staged: true, Modified: false, StatusChar: "A"},
					{Path: "modified_only.go", Staged: false, Modified: true, StatusChar: "M"},
					{Path: "untracked_file.txt", Staged: false, Modified: false, Untracked: true, StatusChar: "??"},
				}
				gitOverlay.SetFiles(files, "main")
				return gitOverlay
			},
			snapshot: "complex_states.txt",
		},
		{
			name: "small_terminal_size",
			setup: func() *overlay.GitStatusOverlay {
				gitOverlay := overlay.NewGitStatusOverlay()
				gitOverlay.SetSize(40, 15) // Small terminal
				files := []overlay.GitFileStatus{
					{Path: "very/long/path/to/some/file.go", Staged: true, StatusChar: "A"},
					{Path: "short.go", Modified: true, StatusChar: "M"},
					{Path: "another/long/path/file.txt", Untracked: true, StatusChar: "??"},
				}
				gitOverlay.SetFiles(files, "feature-branch")
				return gitOverlay
			},
			snapshot: "small_terminal.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitOverlay := tt.setup()
			renderer.CompareComponentWithSnapshot(t, gitOverlay, tt.snapshot)
		})
	}
}

func TestGitStatusOverlay_Navigation(t *testing.T) {
	// Test navigation and selection states
	files := []overlay.GitFileStatus{
		{Path: "first.go", Modified: true, StatusChar: "M"},
		{Path: "second.go", Staged: true, StatusChar: "A"},
		{Path: "third.go", Untracked: true, StatusChar: "??"},
	}

	tests := []struct {
		name     string
		keys     []string
		snapshot string
	}{
		{
			name:     "default_selection",
			keys:     []string{},
			snapshot: "selection_first.txt",
		},
		{
			name:     "second_item_selected",
			keys:     []string{"j"}, // Move down once
			snapshot: "selection_second.txt",
		},
		{
			name:     "third_item_selected",
			keys:     []string{"j", "j"}, // Move down twice
			snapshot: "selection_third.txt",
		},
		{
			name:     "wrap_to_first",
			keys:     []string{"j", "j", "j"}, // Move down past end, should wrap to first
			snapshot: "selection_wrap_first.txt",
		},
		{
			name:     "up_navigation",
			keys:     []string{"k"}, // Move up from first, should wrap to last
			snapshot: "selection_wrap_last.txt",
		},
	}

	renderer := NewTestRenderer().
		SetSnapshotPath("snapshots/git_status/navigation").
		SetDimensions(80, 25).
		DisableColors()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset overlay for each test
			testOverlay := overlay.NewGitStatusOverlay()
			testOverlay.SetSize(80, 25)
			testOverlay.SetFiles(files, "main")

			// Simulate key presses
			for _, key := range tt.keys {
				msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{rune(key[0])}}
				if key == "j" {
					msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}
				} else if key == "k" {
					msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")}
				}
				testOverlay.HandleKeyPress(msg)
			}

			renderer.CompareComponentWithSnapshot(t, testOverlay, tt.snapshot)
		})
	}
}

func TestGitStatusOverlay_StatusMessages(t *testing.T) {
	// Test various status messages
	renderer := NewTestRenderer().
		SetSnapshotPath("snapshots/git_status/messages").
		SetDimensions(80, 25).
		DisableColors()

	tests := []struct {
		name          string
		statusMessage string
		snapshot      string
	}{
		{
			name:          "default_status",
			statusMessage: "Git Status - Press ? for help",
			snapshot:      "default_message.txt",
		},
		{
			name:          "staging_success",
			statusMessage: "Staged app.go",
			snapshot:      "staging_success.txt",
		},
		{
			name:          "staging_error",
			statusMessage: "Error staging file: permission denied",
			snapshot:      "staging_error.txt",
		},
		{
			name:          "push_in_progress",
			statusMessage: "Pushing changes...",
			snapshot:      "push_progress.txt",
		},
		{
			name:          "commit_success",
			statusMessage: "Creating commit...",
			snapshot:      "commit_progress.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitOverlay := overlay.NewGitStatusOverlay()
			gitOverlay.SetSize(80, 25)
			gitOverlay.SetStatusMessage(tt.statusMessage)

			// Add some sample files
			files := []overlay.GitFileStatus{
				{Path: "app.go", Modified: true, StatusChar: "M"},
				{Path: "test.go", Staged: true, StatusChar: "A"},
			}
			gitOverlay.SetFiles(files, "main")

			renderer.CompareComponentWithSnapshot(t, gitOverlay, tt.snapshot)
		})
	}
}

func TestGitStatusOverlay_EdgeCases(t *testing.T) {
	renderer := NewTestRenderer().
		SetSnapshotPath("snapshots/git_status/edge_cases").
		SetDimensions(80, 25).
		DisableColors()

	tests := []struct {
		name     string
		setup    func() *overlay.GitStatusOverlay
		snapshot string
	}{
		{
			name: "no_branch_name",
			setup: func() *overlay.GitStatusOverlay {
				gitOverlay := overlay.NewGitStatusOverlay()
				gitOverlay.SetSize(80, 25)
				files := []overlay.GitFileStatus{
					{Path: "file.go", Modified: true, StatusChar: "M"},
				}
				gitOverlay.SetFiles(files, "") // No branch name
				return gitOverlay
			},
			snapshot: "no_branch.txt",
		},
		{
			name: "very_long_file_paths",
			setup: func() *overlay.GitStatusOverlay {
				gitOverlay := overlay.NewGitStatusOverlay()
				gitOverlay.SetSize(80, 25)
				files := []overlay.GitFileStatus{
					{Path: "src/very/deeply/nested/directory/structure/with/extremely/long/path/filename.go", Modified: true, StatusChar: "M"},
					{Path: "another/incredibly/long/path/that/should/be/truncated/or/wrapped/properly/file.txt", Staged: true, StatusChar: "A"},
				}
				gitOverlay.SetFiles(files, "main")
				return gitOverlay
			},
			snapshot: "long_paths.txt",
		},
		{
			name: "single_file",
			setup: func() *overlay.GitStatusOverlay {
				gitOverlay := overlay.NewGitStatusOverlay()
				gitOverlay.SetSize(80, 25)
				files := []overlay.GitFileStatus{
					{Path: "lonely_file.go", Staged: true, StatusChar: "A"},
				}
				gitOverlay.SetFiles(files, "main")
				return gitOverlay
			},
			snapshot: "single_file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitOverlay := tt.setup()
			renderer.CompareComponentWithSnapshot(t, gitOverlay, tt.snapshot)
		})
	}
}
