package overlay

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBranchChoiceEnum(t *testing.T) {
	// Test that branch choice constants work correctly
	testCases := []struct {
		name     string
		choice   BranchChoice
		expected string
	}{
		{"new branch", BranchChoiceNew, "new"},
		{"current branch", BranchChoiceCurrent, "current"},
		{"existing branch", BranchChoiceExisting, "existing"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.choice) != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, string(tc.choice))
			}
		})
	}
}

func TestBranchStepFlow(t *testing.T) {
	// Create a temporary directory that is a git repository
	tempDir := t.TempDir()
	gitDir := filepath.Join(tempDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Change to the temporary directory for this test
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalDir)

	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	overlay := NewSessionSetupOverlay()
	overlay.sessionName = "test-session"
	overlay.program = "test-program"

	// Test the flow: StepLocation (current) -> StepBranch
	overlay.step = StepLocation
	overlay.locationChoice = "current"

	// Call nextStep to move from location to branch
	overlay.nextStep()

	// Should now be at StepBranch
	if overlay.step != StepBranch {
		t.Errorf("Expected to be at StepBranch, got %v", overlay.step)
	}

	// Should have set repoPath to current directory
	if overlay.repoPath == "" {
		t.Error("Expected repoPath to be set")
	}

	// Test branch choice cycling
	originalChoice := overlay.branchChoice
	if originalChoice != BranchChoiceNew {
		t.Errorf("Expected initial branch choice to be %s, got %s", BranchChoiceNew, originalChoice)
	}

	// Test cycling through choices (Tab behavior)
	testChoices := []BranchChoice{BranchChoiceNew, BranchChoiceCurrent, BranchChoiceExisting, BranchChoiceNew}
	currentChoice := overlay.branchChoice

	for i := 1; i < len(testChoices); i++ {
		// Simulate Tab key cycling
		switch currentChoice {
		case BranchChoiceNew:
			currentChoice = BranchChoiceCurrent
		case BranchChoiceCurrent:
			currentChoice = BranchChoiceExisting
		case BranchChoiceExisting:
			currentChoice = BranchChoiceNew
		}

		expected := testChoices[i]
		if currentChoice != expected {
			t.Errorf("After %d cycles, expected %s, got %s", i, expected, currentChoice)
		}
	}
}

func TestLoadGitBranchesWithEmptyPath(t *testing.T) {
	overlay := NewSessionSetupOverlay()

	// Test with empty repo path
	branches := overlay.loadGitBranches("")
	if len(branches) == 0 {
		t.Error("Expected at least one branch item for empty path")
	}

	// Should return a helpful message
	firstBranch := branches[0]
	if firstBranch.GetID() != "main" {
		t.Errorf("Expected first branch ID to be 'main', got %s", firstBranch.GetID())
	}

	if firstBranch.GetDisplayText() != "main (no repository path provided)" {
		t.Errorf("Expected helpful message for empty path, got %s", firstBranch.GetDisplayText())
	}
}

func TestLoadGitBranchesWithNonGitPath(t *testing.T) {
	overlay := NewSessionSetupOverlay()
	tempDir := t.TempDir()

	// Test with non-git directory
	branches := overlay.loadGitBranches(tempDir)
	if len(branches) == 0 {
		t.Error("Expected at least one branch item for non-git path")
	}

	// Should return a helpful message
	firstBranch := branches[0]
	if firstBranch.GetID() != "main" {
		t.Errorf("Expected first branch ID to be 'main', got %s", firstBranch.GetID())
	}

	expectedText := "main (not a Git repository: " + tempDir + ")"
	if firstBranch.GetDisplayText() != expectedText {
		t.Errorf("Expected helpful message for non-git path, got %s", firstBranch.GetDisplayText())
	}
}

func TestBranchChoiceNextStep(t *testing.T) {
	overlay := NewSessionSetupOverlay()
	overlay.step = StepBranch
	overlay.sessionName = "test"
	overlay.program = "test-program"
	overlay.repoPath = "/tmp" // Set a valid path

	testCases := []struct {
		name               string
		branchChoice       BranchChoice
		expectedStep       SessionSetupStep
		shouldInitSelector bool
	}{
		{"new branch goes to confirm", BranchChoiceNew, StepConfirm, false},
		{"current branch goes to confirm", BranchChoiceCurrent, StepConfirm, false},
		{"existing branch should init selector", BranchChoiceExisting, StepBranch, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			overlay.step = StepBranch
			overlay.branchChoice = tc.branchChoice
			overlay.branchSelector = nil // Reset selector

			overlay.nextStep()

			if overlay.step != tc.expectedStep {
				t.Errorf("Expected step %v, got %v", tc.expectedStep, overlay.step)
			}

			if tc.shouldInitSelector && overlay.branchSelector == nil {
				t.Error("Expected branch selector to be initialized")
			}

			if !tc.shouldInitSelector && overlay.branchSelector != nil {
				t.Error("Expected branch selector to NOT be initialized")
			}
		})
	}
}