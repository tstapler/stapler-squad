package github

import (
	"testing"
)

func TestParseGitHubRef(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantType  RefType
		wantOwner string
		wantRepo  string
		wantPR    int
		wantBranch string
		wantErr   bool
	}{
		// PR URLs
		{
			name:      "PR URL with https",
			input:     "https://github.com/owner/repo/pull/123",
			wantType:  RefTypePR,
			wantOwner: "owner",
			wantRepo:  "repo",
			wantPR:    123,
		},
		{
			name:      "PR URL without https",
			input:     "github.com/owner/repo/pull/456",
			wantType:  RefTypePR,
			wantOwner: "owner",
			wantRepo:  "repo",
			wantPR:    456,
		},
		{
			name:      "PR URL with trailing path",
			input:     "https://github.com/owner/repo/pull/789/files",
			wantType:  RefTypePR,
			wantOwner: "owner",
			wantRepo:  "repo",
			wantPR:    789,
		},

		// Branch URLs
		{
			name:       "Branch URL with https",
			input:      "https://github.com/owner/repo/tree/feature-branch",
			wantType:   RefTypeBranch,
			wantOwner:  "owner",
			wantRepo:   "repo",
			wantBranch: "feature-branch",
		},
		{
			name:       "Branch URL with slashes in branch",
			input:      "https://github.com/owner/repo/tree/feature/my-feature",
			wantType:   RefTypeBranch,
			wantOwner:  "owner",
			wantRepo:   "repo",
			wantBranch: "feature/my-feature",
		},
		{
			name:       "Branch URL without https",
			input:      "github.com/owner/repo/tree/main",
			wantType:   RefTypeBranch,
			wantOwner:  "owner",
			wantRepo:   "repo",
			wantBranch: "main",
		},

		// Repository URLs
		{
			name:      "Repo URL with https",
			input:     "https://github.com/owner/repo",
			wantType:  RefTypeRepo,
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "Repo URL with https and trailing slash",
			input:     "https://github.com/owner/repo/",
			wantType:  RefTypeRepo,
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "Repo URL with .git suffix",
			input:     "https://github.com/owner/repo.git",
			wantType:  RefTypeRepo,
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "Repo URL without https",
			input:     "github.com/owner/repo",
			wantType:  RefTypeRepo,
			wantOwner: "owner",
			wantRepo:  "repo",
		},

		// Shorthand formats
		{
			name:       "Shorthand with branch",
			input:      "owner/repo:feature-branch",
			wantType:   RefTypeBranch,
			wantOwner:  "owner",
			wantRepo:   "repo",
			wantBranch: "feature-branch",
		},
		{
			name:      "Shorthand repo only",
			input:     "owner/repo",
			wantType:  RefTypeRepo,
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "Shorthand with hyphenated names",
			input:     "my-org/my-repo",
			wantType:  RefTypeRepo,
			wantOwner: "my-org",
			wantRepo:  "my-repo",
		},
		{
			name:       "Shorthand with branch containing slashes",
			input:      "owner/repo:feature/my-feature",
			wantType:   RefTypeBranch,
			wantOwner:  "owner",
			wantRepo:   "repo",
			wantBranch: "feature/my-feature",
		},

		// Real-world examples
		{
			name:      "claude-squad repo",
			input:     "https://github.com/anthropics/claude-squad",
			wantType:  RefTypeRepo,
			wantOwner: "anthropics",
			wantRepo:  "claude-squad",
		},
		{
			name:      "claude-squad PR",
			input:     "https://github.com/anthropics/claude-squad/pull/42",
			wantType:  RefTypePR,
			wantOwner: "anthropics",
			wantRepo:  "claude-squad",
			wantPR:    42,
		},

		// Error cases
		{
			name:    "Empty input",
			input:   "",
			wantErr: true,
		},
		{
			name:    "Random text",
			input:   "hello world",
			wantErr: true,
		},
		{
			name:    "Local path",
			input:   "/Users/test/project",
			wantErr: true,
		},
		{
			name:    "Non-GitHub URL",
			input:   "https://gitlab.com/owner/repo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseGitHubRef(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseGitHubRef() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if got.Type != tt.wantType {
				t.Errorf("ParseGitHubRef() Type = %v, want %v", got.Type, tt.wantType)
			}
			if got.Owner != tt.wantOwner {
				t.Errorf("ParseGitHubRef() Owner = %v, want %v", got.Owner, tt.wantOwner)
			}
			if got.Repo != tt.wantRepo {
				t.Errorf("ParseGitHubRef() Repo = %v, want %v", got.Repo, tt.wantRepo)
			}
			if got.PRNumber != tt.wantPR {
				t.Errorf("ParseGitHubRef() PRNumber = %v, want %v", got.PRNumber, tt.wantPR)
			}
			if got.Branch != tt.wantBranch {
				t.Errorf("ParseGitHubRef() Branch = %v, want %v", got.Branch, tt.wantBranch)
			}
		})
	}
}

func TestIsGitHubRef(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// True cases
		{"https://github.com/owner/repo", true},
		{"github.com/owner/repo", true},
		{"owner/repo", true},
		{"owner/repo:branch", true},
		{"https://github.com/owner/repo/pull/123", true},
		{"https://github.com/owner/repo/tree/main", true},
		{"my-org/my-repo", true},

		// False cases
		{"", false},
		{"hello", false},
		{"/local/path", false},
		{"https://gitlab.com/owner/repo", false},
		{"owner", false},
		{"-invalid/repo", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := IsGitHubRef(tt.input); got != tt.want {
				t.Errorf("IsGitHubRef(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParsedGitHubRef_Methods(t *testing.T) {
	t.Run("CloneURL", func(t *testing.T) {
		ref := &ParsedGitHubRef{Owner: "owner", Repo: "repo"}
		want := "https://github.com/owner/repo.git"
		if got := ref.CloneURL(); got != want {
			t.Errorf("CloneURL() = %v, want %v", got, want)
		}
	})

	t.Run("HTMLURL for PR", func(t *testing.T) {
		ref := &ParsedGitHubRef{Type: RefTypePR, Owner: "owner", Repo: "repo", PRNumber: 123}
		want := "https://github.com/owner/repo/pull/123"
		if got := ref.HTMLURL(); got != want {
			t.Errorf("HTMLURL() = %v, want %v", got, want)
		}
	})

	t.Run("HTMLURL for branch", func(t *testing.T) {
		ref := &ParsedGitHubRef{Type: RefTypeBranch, Owner: "owner", Repo: "repo", Branch: "main"}
		want := "https://github.com/owner/repo/tree/main"
		if got := ref.HTMLURL(); got != want {
			t.Errorf("HTMLURL() = %v, want %v", got, want)
		}
	})

	t.Run("HTMLURL for repo", func(t *testing.T) {
		ref := &ParsedGitHubRef{Type: RefTypeRepo, Owner: "owner", Repo: "repo"}
		want := "https://github.com/owner/repo"
		if got := ref.HTMLURL(); got != want {
			t.Errorf("HTMLURL() = %v, want %v", got, want)
		}
	})

	t.Run("DisplayName for PR", func(t *testing.T) {
		ref := &ParsedGitHubRef{Type: RefTypePR, Owner: "owner", Repo: "repo", PRNumber: 123}
		want := "owner/repo#123"
		if got := ref.DisplayName(); got != want {
			t.Errorf("DisplayName() = %v, want %v", got, want)
		}
	})

	t.Run("SuggestedSessionName for PR", func(t *testing.T) {
		ref := &ParsedGitHubRef{Type: RefTypePR, Owner: "owner", Repo: "my-repo", PRNumber: 123}
		want := "pr-123-my-repo"
		if got := ref.SuggestedSessionName(); got != want {
			t.Errorf("SuggestedSessionName() = %v, want %v", got, want)
		}
	})

	t.Run("SuggestedSessionName for branch with slash", func(t *testing.T) {
		ref := &ParsedGitHubRef{Type: RefTypeBranch, Owner: "owner", Repo: "repo", Branch: "feature/test"}
		want := "repo-feature-test"
		if got := ref.SuggestedSessionName(); got != want {
			t.Errorf("SuggestedSessionName() = %v, want %v", got, want)
		}
	})
}

func TestRefType_String(t *testing.T) {
	tests := []struct {
		refType RefType
		want    string
	}{
		{RefTypePR, "PR"},
		{RefTypeBranch, "Branch"},
		{RefTypeRepo, "Repository"},
		{RefType(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.refType.String(); got != tt.want {
				t.Errorf("RefType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
