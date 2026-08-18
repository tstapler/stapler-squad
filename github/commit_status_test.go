package github

import "testing"

// TestNewCommitStatusRequest_ValidatesRequiredFields guards against the primitive-obsession
// failure mode this constructor exists to prevent: a caller passing an invalid state string
// or an empty required field should fail at construction, not surface as an opaque GitHub
// API 422 later.
func TestNewCommitStatusRequest_ValidatesRequiredFields(t *testing.T) {
	tests := []struct {
		name          string
		sha           string
		state         CommitStatusState
		statusContext string
		description   string
		wantErr       bool
	}{
		{
			name:          "valid_pending",
			sha:           "abc123",
			state:         CommitStatusStatePending,
			statusContext: "stapler-squad/review",
			description:   "Review in progress",
		},
		{
			name:          "valid_success",
			sha:           "abc123",
			state:         CommitStatusStateSuccess,
			statusContext: "stapler-squad/review",
			description:   "Review passed",
		},
		{
			name:          "valid_error",
			sha:           "abc123",
			state:         CommitStatusStateError,
			statusContext: "stapler-squad/review",
			description:   "Review errored",
		},
		{
			name:          "valid_failure",
			sha:           "abc123",
			state:         CommitStatusStateFailure,
			statusContext: "stapler-squad/review",
			description:   "Review failed",
		},
		{
			name:          "empty_sha",
			sha:           "",
			state:         CommitStatusStateSuccess,
			statusContext: "stapler-squad/review",
			description:   "Review passed",
			wantErr:       true,
		},
		{
			name:          "empty_status_context",
			sha:           "abc123",
			state:         CommitStatusStateSuccess,
			statusContext: "",
			description:   "Review passed",
			wantErr:       true,
		},
		{
			name:          "empty_description",
			sha:           "abc123",
			state:         CommitStatusStateSuccess,
			statusContext: "stapler-squad/review",
			description:   "",
			wantErr:       true,
		},
		{
			name:          "invalid_state",
			sha:           "abc123",
			state:         CommitStatusState("in_progress"),
			statusContext: "stapler-squad/review",
			description:   "Review passed",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := NewCommitStatusRequest(tt.sha, tt.state, tt.statusContext, tt.description)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewCommitStatusRequest(%q, %q, %q, %q) = %+v, nil; want error", tt.sha, tt.state, tt.statusContext, tt.description, req)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewCommitStatusRequest(%q, %q, %q, %q) returned unexpected error: %v", tt.sha, tt.state, tt.statusContext, tt.description, err)
			}
			if req.SHA != tt.sha || req.State != tt.state || req.StatusContext != tt.statusContext || req.Description != tt.description {
				t.Errorf("NewCommitStatusRequest(...) = %+v, fields do not match input", req)
			}
		})
	}
}
