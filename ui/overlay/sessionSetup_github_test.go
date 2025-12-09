package overlay

import (
	"claude-squad/github"
	"claude-squad/session"
	"testing"
)

// TestGitHubURLDetection verifies that GitHub URLs are properly detected and parsed
func TestGitHubURLDetection(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectGitHub   bool
		expectedType   github.RefType
		expectedOwner  string
		expectedRepo   string
		expectedPR     int
		expectedBranch string
	}{
		{
			name:          "PR URL",
			input:         "https://github.com/owner/repo/pull/123",
			expectGitHub:  true,
			expectedType:  github.RefTypePR,
			expectedOwner: "owner",
			expectedRepo:  "repo",
			expectedPR:    123,
		},
		{
			name:           "Branch URL",
			input:          "https://github.com/owner/repo/tree/feature-branch",
			expectGitHub:   true,
			expectedType:   github.RefTypeBranch,
			expectedOwner:  "owner",
			expectedRepo:   "repo",
			expectedBranch: "feature-branch",
		},
		{
			name:          "Repo URL",
			input:         "https://github.com/owner/repo",
			expectGitHub:  true,
			expectedType:  github.RefTypeRepo,
			expectedOwner: "owner",
			expectedRepo:  "repo",
		},
		{
			name:          "Shorthand repo",
			input:         "owner/repo",
			expectGitHub:  true,
			expectedType:  github.RefTypeRepo,
			expectedOwner: "owner",
			expectedRepo:  "repo",
		},
		{
			name:           "Shorthand with branch",
			input:          "owner/repo:main",
			expectGitHub:   true,
			expectedType:   github.RefTypeBranch,
			expectedOwner:  "owner",
			expectedRepo:   "repo",
			expectedBranch: "main",
		},
		{
			name:         "Regular session name",
			input:        "my-session",
			expectGitHub: false,
		},
		{
			name:         "Path-like string",
			input:        "/path/to/project",
			expectGitHub: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a session setup overlay
			callbacks := SessionSetupCallbacks{
				OnComplete: func(opts session.InstanceOptions) {
					// No-op for testing
				},
			}
			overlay := NewSessionSetupOverlay(callbacks)

			// Simulate setting the input value
			overlay.nameInput = NewTextInputOverlay("Session Name", tt.input)

			// Run detection
			overlay.detectGitHubURL()

			// Verify GitHub detection
			if overlay.isGitHubSession != tt.expectGitHub {
				t.Errorf("Expected isGitHubSession=%v, got %v", tt.expectGitHub, overlay.isGitHubSession)
			}

			if !tt.expectGitHub {
				// No further checks for non-GitHub inputs
				return
			}

			// Verify parsed reference
			if overlay.parsedGitHubRef == nil {
				t.Fatal("Expected parsedGitHubRef to be non-nil for GitHub URL")
			}

			if overlay.parsedGitHubRef.Type != tt.expectedType {
				t.Errorf("Expected type=%v, got %v", tt.expectedType, overlay.parsedGitHubRef.Type)
			}

			if overlay.parsedGitHubRef.Owner != tt.expectedOwner {
				t.Errorf("Expected owner=%s, got %s", tt.expectedOwner, overlay.parsedGitHubRef.Owner)
			}

			if overlay.parsedGitHubRef.Repo != tt.expectedRepo {
				t.Errorf("Expected repo=%s, got %s", tt.expectedRepo, overlay.parsedGitHubRef.Repo)
			}

			if tt.expectedType == github.RefTypePR {
				if overlay.parsedGitHubRef.PRNumber != tt.expectedPR {
					t.Errorf("Expected PR=%d, got %d", tt.expectedPR, overlay.parsedGitHubRef.PRNumber)
				}
				if !overlay.generatePRPrompt {
					t.Error("Expected generatePRPrompt=true for PR URLs")
				}
			}

			if tt.expectedType == github.RefTypeBranch {
				if overlay.parsedGitHubRef.Branch != tt.expectedBranch {
					t.Errorf("Expected branch=%s, got %s", tt.expectedBranch, overlay.parsedGitHubRef.Branch)
				}
			}
		})
	}
}

// TestGitHubSessionMetadata verifies that GitHub metadata is properly passed to session options
func TestGitHubSessionMetadata(t *testing.T) {
	// Skip this test if we can't actually clone (requires network and gh CLI)
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This is a more complex integration test that would require:
	// 1. Mocking the github.GetOrCloneRepository function
	// 2. Verifying the complete flow from detection to session creation
	// For now, we'll test the URL detection in isolation above
	t.Skip("Integration test not yet implemented - requires mocking")
}

// TestSuggestedSessionNames verifies session name suggestions from GitHub refs
func TestSuggestedSessionNames(t *testing.T) {
	tests := []struct {
		input        string
		expectedName string
	}{
		{
			input:        "https://github.com/owner/repo/pull/123",
			expectedName: "pr-123-repo",
		},
		{
			input:        "owner/repo:feature-branch",
			expectedName: "repo-feature-branch",
		},
		{
			input:        "https://github.com/owner/repo",
			expectedName: "repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			parsed, err := github.ParseGitHubRef(tt.input)
			if err != nil {
				t.Fatalf("Failed to parse GitHub ref: %v", err)
			}

			suggestedName := parsed.SuggestedSessionName()
			if suggestedName != tt.expectedName {
				t.Errorf("Expected suggested name=%s, got %s", tt.expectedName, suggestedName)
			}
		})
	}
}
