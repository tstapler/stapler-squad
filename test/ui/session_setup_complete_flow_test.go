package ui

import (
	"testing"

	"claude-squad/ui/overlay"
	tea "github.com/charmbracelet/bubbletea"
)

// TestSessionSetupOverlay_AllSteps_Snapshots tests ALL dialog layers including advanced flows
// This ensures we have visual regression testing for every step the user can reach
func TestSessionSetupOverlay_AllSteps_Snapshots(t *testing.T) {
	renderer := NewTestRenderer().
		SetSnapshotPath("snapshots/session_setup").
		SetDimensions(80, 30).
		DisableColors()

	tests := []struct {
		name     string
		setup    func() *overlay.SessionSetupOverlay
		snapshot string
	}{
		// ===== ADVANCED FLOWS - Repository Selector =====
		{
			name: "repository_selector_initial",
			setup: func() *overlay.SessionSetupOverlay {
				s := overlay.NewSessionSetupOverlay()
				s.SetSize(80, 30)
				// Navigate to repository selector
				s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test-session")})
				s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Basics -> Location
				s.Update(tea.KeyMsg{Type: tea.KeyTab})   // Select "different"
				s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Location -> Repository selector
				return s
			},
			snapshot: "advanced/repository_selector_initial.txt",
		},
		{
			name: "repository_selector_with_input",
			setup: func() *overlay.SessionSetupOverlay {
				s := overlay.NewSessionSetupOverlay()
				s.SetSize(80, 30)
				// Navigate to repository selector and type a path
				s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test-session")})
				s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Basics -> Location
				s.Update(tea.KeyMsg{Type: tea.KeyTab})   // Select "different"
				s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Location -> Repository selector
				// Type a path in the repository selector
				s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("~/projects")})
				return s
			},
			snapshot: "advanced/repository_selector_with_input.txt",
		},

		// ===== ADVANCED FLOWS - Worktree Selector =====
		{
			name: "worktree_selector_initial",
			setup: func() *overlay.SessionSetupOverlay {
				s := overlay.NewSessionSetupOverlay()
				s.SetSize(80, 30)
				// Navigate to worktree selector
				s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test-session")})
				s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Basics -> Location
				s.Update(tea.KeyMsg{Type: tea.KeyTab})   // Select "different"
				s.Update(tea.KeyMsg{Type: tea.KeyTab})   // Select "existing"
				s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Location -> Worktree selector
				return s
			},
			snapshot: "advanced/worktree_selector_initial.txt",
		},

		// ===== ADVANCED FLOWS - Branch Choice =====
		{
			name: "branch_choice_new_selected",
			setup: func() *overlay.SessionSetupOverlay {
				s := overlay.NewSessionSetupOverlay()
				s.SetSize(80, 30)
				// Navigate through current location to branch choice
				s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test-session")})
				s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Basics -> Location
				s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Location (current) -> Branch choice
				// Default is "new" branch
				return s
			},
			snapshot: "advanced/branch_choice_new.txt",
		},
		{
			name: "branch_choice_current_selected",
			setup: func() *overlay.SessionSetupOverlay {
				s := overlay.NewSessionSetupOverlay()
				s.SetSize(80, 30)
				s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test-session")})
				s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Basics -> Location
				s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Location (current) -> Branch choice
				s.Update(tea.KeyMsg{Type: tea.KeyTab})   // Switch to "current" branch
				return s
			},
			snapshot: "advanced/branch_choice_current.txt",
		},
		{
			name: "branch_choice_existing_selected",
			setup: func() *overlay.SessionSetupOverlay {
				s := overlay.NewSessionSetupOverlay()
				s.SetSize(80, 30)
				s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test-session")})
				s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Basics -> Location
				s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Location (current) -> Branch choice
				s.Update(tea.KeyMsg{Type: tea.KeyTab})   // Switch to "current"
				s.Update(tea.KeyMsg{Type: tea.KeyTab})   // Switch to "existing"
				return s
			},
			snapshot: "advanced/branch_choice_existing.txt",
		},

		// ===== ERROR STATES AT DIFFERENT STEPS =====
		{
			name: "error_empty_name_at_basics",
			setup: func() *overlay.SessionSetupOverlay {
				s := overlay.NewSessionSetupOverlay()
				s.SetSize(80, 30)
				// Try to advance without entering a name
				s.Update(tea.KeyMsg{Type: tea.KeyEnter})
				return s
			},
			snapshot: "errors/empty_name_basics.txt",
		},

		// ===== TERMINAL SIZE VARIATIONS FOR ADVANCED FLOWS =====
		{
			name: "repository_selector_small_terminal",
			setup: func() *overlay.SessionSetupOverlay {
				s := overlay.NewSessionSetupOverlay()
				s.SetSize(60, 20) // Smaller terminal
				s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})
				s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Basics -> Location
				s.Update(tea.KeyMsg{Type: tea.KeyTab})   // Select "different"
				s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Location -> Repository selector
				return s
			},
			snapshot: "responsive/repository_selector_small.txt",
		},
		{
			name: "branch_choice_large_terminal",
			setup: func() *overlay.SessionSetupOverlay {
				s := overlay.NewSessionSetupOverlay()
				s.SetSize(100, 35) // Larger terminal
				s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})
				s.Update(tea.KeyMsg{Type: tea.KeyEnter})
				s.Update(tea.KeyMsg{Type: tea.KeyEnter})
				return s
			},
			snapshot: "responsive/branch_choice_large.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionSetup := tt.setup()
			renderer.CompareComponentWithSnapshot(t, sessionSetup, tt.snapshot)
		})
	}
}

// TestSessionSetupOverlay_CompleteFlows tests complete user journeys through the dialog
func TestSessionSetupOverlay_CompleteFlows(t *testing.T) {
	renderer := NewTestRenderer().
		SetSnapshotPath("snapshots/session_setup").
		SetDimensions(80, 30).
		DisableColors()

	tests := []struct {
		name        string
		description string
		keys        []tea.KeyMsg
		snapshot    string
	}{
		{
			name:        "complete_flow_with_different_repo",
			description: "Complete flow selecting different repository",
			keys: []tea.KeyMsg{
				{Type: tea.KeyRunes, Runes: []rune("my-project")},
				{Type: tea.KeyEnter}, // Basics -> Location
				{Type: tea.KeyTab},   // Select "different"
				{Type: tea.KeyEnter}, // Location -> Repository selector (step StepRepository)
			},
			snapshot: "flows/complete_different_repo_at_selector.txt",
		},
		{
			name:        "complete_flow_with_existing_worktree",
			description: "Complete flow selecting existing worktree",
			keys: []tea.KeyMsg{
				{Type: tea.KeyRunes, Runes: []rune("worktree-session")},
				{Type: tea.KeyEnter}, // Basics -> Location
				{Type: tea.KeyTab},   // "different"
				{Type: tea.KeyTab},   // "existing"
				{Type: tea.KeyEnter}, // Location -> Worktree selector
			},
			snapshot: "flows/complete_existing_worktree_at_selector.txt",
		},
		{
			name:        "complete_flow_new_branch",
			description: "Complete flow creating new branch",
			keys: []tea.KeyMsg{
				{Type: tea.KeyRunes, Runes: []rune("feature-session")},
				{Type: tea.KeyEnter}, // Basics -> Location
				{Type: tea.KeyEnter}, // Location (current) -> Branch choice
				// Default is "new" branch - stay there
			},
			snapshot: "flows/complete_new_branch.txt",
		},
		{
			name:        "complete_flow_use_current_branch",
			description: "Complete flow using current branch",
			keys: []tea.KeyMsg{
				{Type: tea.KeyRunes, Runes: []rune("current-branch-session")},
				{Type: tea.KeyEnter}, // Basics -> Location
				{Type: tea.KeyEnter}, // Location (current) -> Branch choice
				{Type: tea.KeyTab},   // Select "current" branch
			},
			snapshot: "flows/complete_current_branch.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionSetup := overlay.NewSessionSetupOverlay()
			sessionSetup.SetSize(80, 30)

			// Process all key events
			for _, key := range tt.keys {
				sessionSetup.Update(key)
			}

			renderer.CompareComponentWithSnapshot(t, sessionSetup, tt.snapshot)
		})
	}
}

// TestSessionSetupOverlay_BackNavigation tests backward navigation through all steps
func TestSessionSetupOverlay_BackNavigation(t *testing.T) {
	renderer := NewTestRenderer().
		SetSnapshotPath("snapshots/session_setup").
		SetDimensions(80, 30).
		DisableColors()

	tests := []struct {
		name     string
		keys     []tea.KeyMsg
		snapshot string
	}{
		{
			name: "back_from_branch_to_location",
			keys: []tea.KeyMsg{
				{Type: tea.KeyRunes, Runes: []rune("test")},
				{Type: tea.KeyEnter}, // Basics -> Location
				{Type: tea.KeyEnter}, // Location -> Branch
				{Type: tea.KeyEscape}, // Branch -> Back to Location (via prevStep)
			},
			snapshot: "navigation/back_branch_to_location.txt",
		},
		{
			name: "back_from_confirm_to_branch",
			keys: []tea.KeyMsg{
				{Type: tea.KeyRunes, Runes: []rune("test")},
				{Type: tea.KeyEnter}, // Basics -> Location
				{Type: tea.KeyEnter}, // Location -> Branch
				{Type: tea.KeyEnter}, // Branch -> Confirm
				{Type: tea.KeyEscape}, // Confirm -> Back to Branch (via prevStep)
			},
			snapshot: "navigation/back_confirm_to_branch.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionSetup := overlay.NewSessionSetupOverlay()
			sessionSetup.SetSize(80, 30)

			// Process all key events
			for _, key := range tt.keys {
				sessionSetup.Update(key)
			}

			renderer.CompareComponentWithSnapshot(t, sessionSetup, tt.snapshot)
		})
	}
}
