package github

import (
	"testing"
)

func TestParseGitHubRef(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantType   RefType
		wantOwner  string
		wantRepo   string
		wantPR     int
		wantBranch string
		wantErr    bool
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
			name:      "stapler-squad repo",
			input:     "https://github.com/tstapler/stapler-squad",
			wantType:  RefTypeRepo,
			wantOwner: "tstapler",
			wantRepo:  "stapler-squad",
		},
		{
			name:      "stapler-squad PR",
			input:     "https://github.com/tstapler/stapler-squad/pull/42",
			wantType:  RefTypePR,
			wantOwner: "tstapler",
			wantRepo:  "stapler-squad",
			wantPR:    42,
		},

		// SSH URLs
		{
			name:      "SSH URL basic",
			input:     "git@github.com:owner/repo.git",
			wantType:  RefTypeRepo,
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "SSH URL without .git",
			input:     "git@github.com:owner/repo",
			wantType:  RefTypeRepo,
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "SSH protocol URL",
			input:     "ssh://git@github.com/owner/repo.git",
			wantType:  RefTypeRepo,
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "SSH protocol URL without .git",
			input:     "ssh://git@github.com/owner/repo",
			wantType:  RefTypeRepo,
			wantOwner: "owner",
			wantRepo:  "repo",
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
		{RefTypeFile, "File"},
		{RefTypeCommit, "Commit"},
		{RefTypeIssue, "Issue"},
		{RefTypeCompare, "Compare"},
		{RefTypeRelease, "Release"},
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

// TestParseGitHubRef_ExtendedFormats tests the new URL formats added in the enhanced parser
func TestParseGitHubRef_ExtendedFormats(t *testing.T) {
	t.Run("File URLs", func(t *testing.T) {
		tests := []struct {
			name       string
			input      string
			wantOwner  string
			wantRepo   string
			wantBranch string
			wantPath   string
			wantLine   int
			wantEnd    int
		}{
			{
				name:       "Basic file URL",
				input:      "https://github.com/owner/repo/blob/main/src/file.go",
				wantOwner:  "owner",
				wantRepo:   "repo",
				wantBranch: "main",
				wantPath:   "src/file.go",
			},
			{
				name:       "File URL with line number",
				input:      "https://github.com/owner/repo/blob/main/file.go#L42",
				wantOwner:  "owner",
				wantRepo:   "repo",
				wantBranch: "main",
				wantPath:   "file.go",
				wantLine:   42,
			},
			{
				name:       "File URL with line range",
				input:      "https://github.com/owner/repo/blob/main/file.go#L10-L20",
				wantOwner:  "owner",
				wantRepo:   "repo",
				wantBranch: "main",
				wantPath:   "file.go",
				wantLine:   10,
				wantEnd:    20,
			},
			{
				name:       "File URL without https",
				input:      "github.com/owner/repo/blob/feature-branch/path/to/file.ts",
				wantOwner:  "owner",
				wantRepo:   "repo",
				wantBranch: "feature-branch",
				wantPath:   "path/to/file.ts",
			},
			{
				name:       "Deep nested file path",
				input:      "https://github.com/tstapler/stapler-squad/blob/main/ui/overlay/sessionSetup.go#L100-L150",
				wantOwner:  "tstapler",
				wantRepo:   "stapler-squad",
				wantBranch: "main",
				wantPath:   "ui/overlay/sessionSetup.go",
				wantLine:   100,
				wantEnd:    150,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := ParseGitHubRef(tt.input)
				if err != nil {
					t.Fatalf("ParseGitHubRef() unexpected error: %v", err)
				}
				if got.Type != RefTypeFile {
					t.Errorf("Type = %v, want RefTypeFile", got.Type)
				}
				if got.Owner != tt.wantOwner {
					t.Errorf("Owner = %v, want %v", got.Owner, tt.wantOwner)
				}
				if got.Repo != tt.wantRepo {
					t.Errorf("Repo = %v, want %v", got.Repo, tt.wantRepo)
				}
				if got.Branch != tt.wantBranch {
					t.Errorf("Branch = %v, want %v", got.Branch, tt.wantBranch)
				}
				if got.FilePath != tt.wantPath {
					t.Errorf("FilePath = %v, want %v", got.FilePath, tt.wantPath)
				}
				if got.LineStart != tt.wantLine {
					t.Errorf("LineStart = %v, want %v", got.LineStart, tt.wantLine)
				}
				if got.LineEnd != tt.wantEnd {
					t.Errorf("LineEnd = %v, want %v", got.LineEnd, tt.wantEnd)
				}
			})
		}
	})

	t.Run("Commit URLs", func(t *testing.T) {
		tests := []struct {
			name      string
			input     string
			wantOwner string
			wantRepo  string
			wantSHA   string
		}{
			{
				name:      "Full commit SHA",
				input:     "https://github.com/owner/repo/commit/abc123def456789",
				wantOwner: "owner",
				wantRepo:  "repo",
				wantSHA:   "abc123def456789",
			},
			{
				name:      "Short commit SHA",
				input:     "https://github.com/owner/repo/commit/abc123d",
				wantOwner: "owner",
				wantRepo:  "repo",
				wantSHA:   "abc123d",
			},
			{
				name:      "Commit URL without https",
				input:     "github.com/owner/repo/commit/1234567890abcdef",
				wantOwner: "owner",
				wantRepo:  "repo",
				wantSHA:   "1234567890abcdef",
			},
			{
				name:      "Real commit URL",
				input:     "https://github.com/tstapler/stapler-squad/commit/91c1897e991cb2afe0f6a8e8e592c91dede",
				wantOwner: "tstapler",
				wantRepo:  "stapler-squad",
				wantSHA:   "91c1897e991cb2afe0f6a8e8e592c91dede",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := ParseGitHubRef(tt.input)
				if err != nil {
					t.Fatalf("ParseGitHubRef() unexpected error: %v", err)
				}
				if got.Type != RefTypeCommit {
					t.Errorf("Type = %v, want RefTypeCommit", got.Type)
				}
				if got.Owner != tt.wantOwner {
					t.Errorf("Owner = %v, want %v", got.Owner, tt.wantOwner)
				}
				if got.Repo != tt.wantRepo {
					t.Errorf("Repo = %v, want %v", got.Repo, tt.wantRepo)
				}
				if got.CommitSHA != tt.wantSHA {
					t.Errorf("CommitSHA = %v, want %v", got.CommitSHA, tt.wantSHA)
				}
			})
		}
	})

	t.Run("Issue URLs", func(t *testing.T) {
		tests := []struct {
			name      string
			input     string
			wantOwner string
			wantRepo  string
			wantIssue int
		}{
			{
				name:      "Basic issue URL",
				input:     "https://github.com/owner/repo/issues/42",
				wantOwner: "owner",
				wantRepo:  "repo",
				wantIssue: 42,
			},
			{
				name:      "Issue URL without https",
				input:     "github.com/owner/repo/issues/123",
				wantOwner: "owner",
				wantRepo:  "repo",
				wantIssue: 123,
			},
			{
				name:      "Real issue URL",
				input:     "https://github.com/tstapler/stapler-squad/issues/99",
				wantOwner: "tstapler",
				wantRepo:  "stapler-squad",
				wantIssue: 99,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := ParseGitHubRef(tt.input)
				if err != nil {
					t.Fatalf("ParseGitHubRef() unexpected error: %v", err)
				}
				if got.Type != RefTypeIssue {
					t.Errorf("Type = %v, want RefTypeIssue", got.Type)
				}
				if got.Owner != tt.wantOwner {
					t.Errorf("Owner = %v, want %v", got.Owner, tt.wantOwner)
				}
				if got.Repo != tt.wantRepo {
					t.Errorf("Repo = %v, want %v", got.Repo, tt.wantRepo)
				}
				if got.IssueNumber != tt.wantIssue {
					t.Errorf("IssueNumber = %v, want %v", got.IssueNumber, tt.wantIssue)
				}
			})
		}
	})

	t.Run("Compare URLs", func(t *testing.T) {
		tests := []struct {
			name      string
			input     string
			wantOwner string
			wantRepo  string
			wantBase  string
			wantHead  string
		}{
			{
				name:      "Basic compare URL",
				input:     "https://github.com/owner/repo/compare/main...feature",
				wantOwner: "owner",
				wantRepo:  "repo",
				wantBase:  "main",
				wantHead:  "feature",
			},
			{
				name:      "Compare URL without https",
				input:     "github.com/owner/repo/compare/develop...release",
				wantOwner: "owner",
				wantRepo:  "repo",
				wantBase:  "develop",
				wantHead:  "release",
			},
			{
				name:      "Compare with feature branch",
				input:     "https://github.com/tstapler/stapler-squad/compare/main...feature/new-url-parser",
				wantOwner: "tstapler",
				wantRepo:  "stapler-squad",
				wantBase:  "main",
				wantHead:  "feature/new-url-parser",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := ParseGitHubRef(tt.input)
				if err != nil {
					t.Fatalf("ParseGitHubRef() unexpected error: %v", err)
				}
				if got.Type != RefTypeCompare {
					t.Errorf("Type = %v, want RefTypeCompare", got.Type)
				}
				if got.Owner != tt.wantOwner {
					t.Errorf("Owner = %v, want %v", got.Owner, tt.wantOwner)
				}
				if got.Repo != tt.wantRepo {
					t.Errorf("Repo = %v, want %v", got.Repo, tt.wantRepo)
				}
				if got.BaseBranch != tt.wantBase {
					t.Errorf("BaseBranch = %v, want %v", got.BaseBranch, tt.wantBase)
				}
				if got.HeadBranch != tt.wantHead {
					t.Errorf("HeadBranch = %v, want %v", got.HeadBranch, tt.wantHead)
				}
			})
		}
	})

	t.Run("Release URLs", func(t *testing.T) {
		tests := []struct {
			name      string
			input     string
			wantOwner string
			wantRepo  string
			wantTag   string
		}{
			{
				name:      "Basic release URL",
				input:     "https://github.com/owner/repo/releases/tag/v1.0.0",
				wantOwner: "owner",
				wantRepo:  "repo",
				wantTag:   "v1.0.0",
			},
			{
				name:      "Release URL without https",
				input:     "github.com/owner/repo/releases/tag/v2.1.3-beta",
				wantOwner: "owner",
				wantRepo:  "repo",
				wantTag:   "v2.1.3-beta",
			},
			{
				name:      "Real release URL",
				input:     "https://github.com/tstapler/stapler-squad/releases/tag/v0.1.0",
				wantOwner: "tstapler",
				wantRepo:  "stapler-squad",
				wantTag:   "v0.1.0",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := ParseGitHubRef(tt.input)
				if err != nil {
					t.Fatalf("ParseGitHubRef() unexpected error: %v", err)
				}
				if got.Type != RefTypeRelease {
					t.Errorf("Type = %v, want RefTypeRelease", got.Type)
				}
				if got.Owner != tt.wantOwner {
					t.Errorf("Owner = %v, want %v", got.Owner, tt.wantOwner)
				}
				if got.Repo != tt.wantRepo {
					t.Errorf("Repo = %v, want %v", got.Repo, tt.wantRepo)
				}
				if got.Tag != tt.wantTag {
					t.Errorf("Tag = %v, want %v", got.Tag, tt.wantTag)
				}
			})
		}
	})
}

// TestExtendedRefMethods tests the methods for the new ref types
func TestExtendedRefMethods(t *testing.T) {
	t.Run("HTMLURL for File", func(t *testing.T) {
		ref := &ParsedGitHubRef{Type: RefTypeFile, Owner: "owner", Repo: "repo", Branch: "main", FilePath: "src/file.go"}
		want := "https://github.com/owner/repo/blob/main/src/file.go"
		if got := ref.HTMLURL(); got != want {
			t.Errorf("HTMLURL() = %v, want %v", got, want)
		}
	})

	t.Run("HTMLURL for File with line", func(t *testing.T) {
		ref := &ParsedGitHubRef{Type: RefTypeFile, Owner: "owner", Repo: "repo", Branch: "main", FilePath: "file.go", LineStart: 42}
		want := "https://github.com/owner/repo/blob/main/file.go#L42"
		if got := ref.HTMLURL(); got != want {
			t.Errorf("HTMLURL() = %v, want %v", got, want)
		}
	})

	t.Run("HTMLURL for File with line range", func(t *testing.T) {
		ref := &ParsedGitHubRef{Type: RefTypeFile, Owner: "owner", Repo: "repo", Branch: "main", FilePath: "file.go", LineStart: 10, LineEnd: 20}
		want := "https://github.com/owner/repo/blob/main/file.go#L10-L20"
		if got := ref.HTMLURL(); got != want {
			t.Errorf("HTMLURL() = %v, want %v", got, want)
		}
	})

	t.Run("HTMLURL for Commit", func(t *testing.T) {
		ref := &ParsedGitHubRef{Type: RefTypeCommit, Owner: "owner", Repo: "repo", CommitSHA: "abc123def"}
		want := "https://github.com/owner/repo/commit/abc123def"
		if got := ref.HTMLURL(); got != want {
			t.Errorf("HTMLURL() = %v, want %v", got, want)
		}
	})

	t.Run("HTMLURL for Issue", func(t *testing.T) {
		ref := &ParsedGitHubRef{Type: RefTypeIssue, Owner: "owner", Repo: "repo", IssueNumber: 42}
		want := "https://github.com/owner/repo/issues/42"
		if got := ref.HTMLURL(); got != want {
			t.Errorf("HTMLURL() = %v, want %v", got, want)
		}
	})

	t.Run("HTMLURL for Compare", func(t *testing.T) {
		ref := &ParsedGitHubRef{Type: RefTypeCompare, Owner: "owner", Repo: "repo", BaseBranch: "main", HeadBranch: "feature"}
		want := "https://github.com/owner/repo/compare/main...feature"
		if got := ref.HTMLURL(); got != want {
			t.Errorf("HTMLURL() = %v, want %v", got, want)
		}
	})

	t.Run("HTMLURL for Release", func(t *testing.T) {
		ref := &ParsedGitHubRef{Type: RefTypeRelease, Owner: "owner", Repo: "repo", Tag: "v1.0.0"}
		want := "https://github.com/owner/repo/releases/tag/v1.0.0"
		if got := ref.HTMLURL(); got != want {
			t.Errorf("HTMLURL() = %v, want %v", got, want)
		}
	})

	t.Run("DisplayName for Commit", func(t *testing.T) {
		ref := &ParsedGitHubRef{Type: RefTypeCommit, Owner: "owner", Repo: "repo", CommitSHA: "abc123def456"}
		want := "owner/repo@abc123d" // Should be truncated to 7 chars
		if got := ref.DisplayName(); got != want {
			t.Errorf("DisplayName() = %v, want %v", got, want)
		}
	})

	t.Run("DisplayName for Issue", func(t *testing.T) {
		ref := &ParsedGitHubRef{Type: RefTypeIssue, Owner: "owner", Repo: "repo", IssueNumber: 42}
		want := "owner/repo#42 (issue)"
		if got := ref.DisplayName(); got != want {
			t.Errorf("DisplayName() = %v, want %v", got, want)
		}
	})

	t.Run("SuggestedSessionName for File", func(t *testing.T) {
		ref := &ParsedGitHubRef{Type: RefTypeFile, Owner: "owner", Repo: "my-repo", Branch: "main", FilePath: "src/components/Button.tsx"}
		want := "my-repo-main-Button"
		if got := ref.SuggestedSessionName(); got != want {
			t.Errorf("SuggestedSessionName() = %v, want %v", got, want)
		}
	})

	t.Run("SuggestedSessionName for Commit", func(t *testing.T) {
		ref := &ParsedGitHubRef{Type: RefTypeCommit, Owner: "owner", Repo: "my-repo", CommitSHA: "abc123def456"}
		want := "my-repo-commit-abc123d"
		if got := ref.SuggestedSessionName(); got != want {
			t.Errorf("SuggestedSessionName() = %v, want %v", got, want)
		}
	})

	t.Run("SuggestedSessionName for Issue", func(t *testing.T) {
		ref := &ParsedGitHubRef{Type: RefTypeIssue, Owner: "owner", Repo: "my-repo", IssueNumber: 42}
		want := "issue-42-my-repo"
		if got := ref.SuggestedSessionName(); got != want {
			t.Errorf("SuggestedSessionName() = %v, want %v", got, want)
		}
	})

	t.Run("SuggestedSessionName for Compare", func(t *testing.T) {
		ref := &ParsedGitHubRef{Type: RefTypeCompare, Owner: "owner", Repo: "my-repo", BaseBranch: "main", HeadBranch: "feature/test"}
		want := "my-repo-main-vs-feature-test"
		if got := ref.SuggestedSessionName(); got != want {
			t.Errorf("SuggestedSessionName() = %v, want %v", got, want)
		}
	})

	t.Run("SuggestedSessionName for Release", func(t *testing.T) {
		ref := &ParsedGitHubRef{Type: RefTypeRelease, Owner: "owner", Repo: "my-repo", Tag: "v1.0.0"}
		want := "my-repo-v1.0.0"
		if got := ref.SuggestedSessionName(); got != want {
			t.Errorf("SuggestedSessionName() = %v, want %v", got, want)
		}
	})
}

// TestIsGitHubRef_Extended tests the extended IsGitHubRef detection
func TestIsGitHubRef_Extended(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// SSH URLs should be recognized
		{"git@github.com:owner/repo.git", true},
		{"git@github.com:owner/repo", true},
		{"ssh://git@github.com/owner/repo.git", true},
		{"ssh://git@github.com/owner/repo", true},

		// Extended URL formats
		{"https://github.com/owner/repo/blob/main/file.go", true},
		{"https://github.com/owner/repo/commit/abc123", true},
		{"https://github.com/owner/repo/issues/42", true},
		{"https://github.com/owner/repo/compare/main...feature", true},
		{"https://github.com/owner/repo/releases/tag/v1.0", true},

		// Non-GitHub SSH should not match
		{"git@gitlab.com:owner/repo.git", false},
		{"ssh://git@bitbucket.org/owner/repo", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := IsGitHubRef(tt.input); got != tt.want {
				t.Errorf("IsGitHubRef(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
