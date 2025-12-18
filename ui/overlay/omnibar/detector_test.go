package omnibar

import (
	"claude-squad/github"
	"testing"
)

func TestLocalPathDetector_Detect(t *testing.T) {
	d := &LocalPathDetector{}

	tests := []struct {
		name     string
		input    string
		wantType InputType
		wantNil  bool
	}{
		{
			name:     "absolute path",
			input:    "/Users/test/projects/myapp",
			wantType: InputTypeLocalPath,
		},
		{
			name:     "home directory path",
			input:    "~/projects/myapp",
			wantType: InputTypeLocalPath,
		},
		{
			name:     "relative path",
			input:    "./myapp",
			wantType: InputTypeLocalPath,
		},
		{
			name:     "parent relative path",
			input:    "../other-project",
			wantType: InputTypeLocalPath,
		},
		{
			name:     "current directory",
			input:    ".",
			wantType: InputTypeLocalPath,
		},
		{
			name:    "github url",
			input:   "https://github.com/owner/repo",
			wantNil: true,
		},
		{
			name:    "github shorthand",
			input:   "owner/repo",
			wantNil: true,
		},
		{
			name:    "empty input",
			input:   "",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.Detect(tt.input)
			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Errorf("expected non-nil result")
				return
			}
			if result.Type != tt.wantType {
				t.Errorf("expected type %v, got %v", tt.wantType, result.Type)
			}
		})
	}
}

func TestPathWithBranchDetector_Detect(t *testing.T) {
	d := &PathWithBranchDetector{}

	tests := []struct {
		name       string
		input      string
		wantType   InputType
		wantPath   string
		wantBranch string
		wantNil    bool
	}{
		{
			name:       "path with branch",
			input:      "/path/to/repo@feature-branch",
			wantType:   InputTypePathWithBranch,
			wantPath:   "/path/to/repo",
			wantBranch: "feature-branch",
		},
		{
			name:       "home path with branch",
			input:      "~/projects/myapp@main",
			wantType:   InputTypePathWithBranch,
			wantPath:   "~/projects/myapp",
			wantBranch: "main",
		},
		{
			name:    "path without branch",
			input:   "/path/to/repo",
			wantNil: true,
		},
		{
			name:    "github url",
			input:   "https://github.com/owner/repo",
			wantNil: true,
		},
		{
			name:    "invalid branch with spaces",
			input:   "/path/to/repo@invalid branch",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.Detect(tt.input)
			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Errorf("expected non-nil result")
				return
			}
			if result.Type != tt.wantType {
				t.Errorf("expected type %v, got %v", tt.wantType, result.Type)
			}
			if result.LocalPath != tt.wantPath {
				t.Errorf("expected path %q, got %q", tt.wantPath, result.LocalPath)
			}
			if result.Branch != tt.wantBranch {
				t.Errorf("expected branch %q, got %q", tt.wantBranch, result.Branch)
			}
		})
	}
}

func TestGitHubPRDetector_Detect(t *testing.T) {
	d := &GitHubPRDetector{}

	tests := []struct {
		name      string
		input     string
		wantType  InputType
		wantOwner string
		wantRepo  string
		wantPR    int
		wantNil   bool
	}{
		{
			name:      "pr url",
			input:     "https://github.com/owner/repo/pull/123",
			wantType:  InputTypeGitHubPR,
			wantOwner: "owner",
			wantRepo:  "repo",
			wantPR:    123,
		},
		{
			name:      "pr url without https",
			input:     "github.com/owner/repo/pull/456",
			wantType:  InputTypeGitHubPR,
			wantOwner: "owner",
			wantRepo:  "repo",
			wantPR:    456,
		},
		{
			name:    "branch url",
			input:   "https://github.com/owner/repo/tree/main",
			wantNil: true,
		},
		{
			name:    "repo url",
			input:   "https://github.com/owner/repo",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.Detect(tt.input)
			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Errorf("expected non-nil result")
				return
			}
			if result.Type != tt.wantType {
				t.Errorf("expected type %v, got %v", tt.wantType, result.Type)
			}
			if result.GitHubRef.Owner != tt.wantOwner {
				t.Errorf("expected owner %q, got %q", tt.wantOwner, result.GitHubRef.Owner)
			}
			if result.GitHubRef.Repo != tt.wantRepo {
				t.Errorf("expected repo %q, got %q", tt.wantRepo, result.GitHubRef.Repo)
			}
			if result.GitHubRef.PRNumber != tt.wantPR {
				t.Errorf("expected PR %d, got %d", tt.wantPR, result.GitHubRef.PRNumber)
			}
		})
	}
}

func TestGitHubBranchDetector_Detect(t *testing.T) {
	d := &GitHubBranchDetector{}

	tests := []struct {
		name       string
		input      string
		wantType   InputType
		wantBranch string
		wantNil    bool
	}{
		{
			name:       "branch url",
			input:      "https://github.com/owner/repo/tree/feature-branch",
			wantType:   InputTypeGitHubBranch,
			wantBranch: "feature-branch",
		},
		{
			name:       "branch url with slashes",
			input:      "https://github.com/owner/repo/tree/feature/nested/branch",
			wantType:   InputTypeGitHubBranch,
			wantBranch: "feature/nested/branch",
		},
		{
			name:    "pr url",
			input:   "https://github.com/owner/repo/pull/123",
			wantNil: true,
		},
		{
			name:    "repo url",
			input:   "https://github.com/owner/repo",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.Detect(tt.input)
			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Errorf("expected non-nil result")
				return
			}
			if result.Type != tt.wantType {
				t.Errorf("expected type %v, got %v", tt.wantType, result.Type)
			}
			if result.Branch != tt.wantBranch {
				t.Errorf("expected branch %q, got %q", tt.wantBranch, result.Branch)
			}
		})
	}
}

func TestGitHubRepoDetector_Detect(t *testing.T) {
	d := &GitHubRepoDetector{}

	tests := []struct {
		name      string
		input     string
		wantType  InputType
		wantOwner string
		wantRepo  string
		wantNil   bool
	}{
		{
			name:      "repo url",
			input:     "https://github.com/owner/repo",
			wantType:  InputTypeGitHubRepo,
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "repo url with trailing slash",
			input:     "https://github.com/owner/repo/",
			wantType:  InputTypeGitHubRepo,
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:    "pr url",
			input:   "https://github.com/owner/repo/pull/123",
			wantNil: true,
		},
		{
			name:    "branch url",
			input:   "https://github.com/owner/repo/tree/main",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.Detect(tt.input)
			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Errorf("expected non-nil result")
				return
			}
			if result.Type != tt.wantType {
				t.Errorf("expected type %v, got %v", tt.wantType, result.Type)
			}
			if result.GitHubRef.Owner != tt.wantOwner {
				t.Errorf("expected owner %q, got %q", tt.wantOwner, result.GitHubRef.Owner)
			}
			if result.GitHubRef.Repo != tt.wantRepo {
				t.Errorf("expected repo %q, got %q", tt.wantRepo, result.GitHubRef.Repo)
			}
		})
	}
}

func TestGitHubShorthandDetector_Detect(t *testing.T) {
	d := &GitHubShorthandDetector{}

	tests := []struct {
		name       string
		input      string
		wantType   InputType
		wantOwner  string
		wantRepo   string
		wantBranch string
		wantNil    bool
	}{
		{
			name:      "shorthand repo",
			input:     "owner/repo",
			wantType:  InputTypeGitHubShorthand,
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:       "shorthand with branch",
			input:      "owner/repo:feature-branch",
			wantType:   InputTypeGitHubBranch,
			wantOwner:  "owner",
			wantRepo:   "repo",
			wantBranch: "feature-branch",
		},
		{
			name:    "github url",
			input:   "https://github.com/owner/repo",
			wantNil: true,
		},
		{
			name:    "absolute path",
			input:   "/path/to/repo",
			wantNil: true,
		},
		{
			name:    "home path",
			input:   "~/projects/repo",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.Detect(tt.input)
			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Errorf("expected non-nil result")
				return
			}
			if result.Type != tt.wantType {
				t.Errorf("expected type %v, got %v", tt.wantType, result.Type)
			}
			if result.GitHubRef.Owner != tt.wantOwner {
				t.Errorf("expected owner %q, got %q", tt.wantOwner, result.GitHubRef.Owner)
			}
			if result.GitHubRef.Repo != tt.wantRepo {
				t.Errorf("expected repo %q, got %q", tt.wantRepo, result.GitHubRef.Repo)
			}
			if tt.wantBranch != "" && result.Branch != tt.wantBranch {
				t.Errorf("expected branch %q, got %q", tt.wantBranch, result.Branch)
			}
		})
	}
}

func TestRegistry_Detect(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantType InputType
	}{
		{
			name:     "pr url takes priority",
			input:    "https://github.com/owner/repo/pull/123",
			wantType: InputTypeGitHubPR,
		},
		{
			name:     "branch url",
			input:    "https://github.com/owner/repo/tree/main",
			wantType: InputTypeGitHubBranch,
		},
		{
			name:     "repo url",
			input:    "https://github.com/owner/repo",
			wantType: InputTypeGitHubRepo,
		},
		{
			name:     "shorthand",
			input:    "owner/repo",
			wantType: InputTypeGitHubShorthand,
		},
		{
			name:     "path with branch",
			input:    "/path/to/repo@main",
			wantType: InputTypePathWithBranch,
		},
		{
			name:     "local path",
			input:    "/path/to/repo",
			wantType: InputTypeLocalPath,
		},
		{
			name:     "home path",
			input:    "~/projects/myapp",
			wantType: InputTypeLocalPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DefaultRegistry.Detect(tt.input)
			if result.Type != tt.wantType {
				t.Errorf("expected type %v, got %v", tt.wantType, result.Type)
			}
		})
	}
}

func TestIsValidBranchName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid simple", "main", true},
		{"valid with hyphen", "feature-branch", true},
		{"valid with slash", "feature/add-login", true},
		{"valid with numbers", "v1.2.3", true},
		{"empty", "", false},
		{"starts with dot", ".hidden", false},
		{"ends with lock", "branch.lock", false},
		{"contains space", "my branch", false},
		{"contains tilde", "branch~1", false},
		{"contains caret", "branch^1", false},
		{"contains colon", "branch:1", false},
		{"contains double dot", "branch..main", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidBranchName(tt.input)
			if got != tt.want {
				t.Errorf("isValidBranchName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestInputType_String(t *testing.T) {
	tests := []struct {
		input InputType
		want  string
	}{
		{InputTypeLocalPath, "Local Path"},
		{InputTypePathWithBranch, "Path + Branch"},
		{InputTypeGitHubPR, "GitHub PR"},
		{InputTypeGitHubBranch, "GitHub Branch"},
		{InputTypeGitHubRepo, "GitHub Repository"},
		{InputTypeGitHubShorthand, "GitHub Shorthand"},
		{InputTypeUnknown, "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.input.String()
			if got != tt.want {
				t.Errorf("InputType.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInputType_Icon(t *testing.T) {
	tests := []struct {
		input InputType
		want  string
	}{
		{InputTypeLocalPath, "📁"},
		{InputTypePathWithBranch, "📁🌿"},
		{InputTypeGitHubPR, "🔀"},
		{InputTypeGitHubBranch, "🌿"},
		{InputTypeGitHubRepo, "📦"},
		{InputTypeGitHubShorthand, "📦"},
		{InputTypeUnknown, "❓"},
	}

	for _, tt := range tests {
		t.Run(tt.input.String(), func(t *testing.T) {
			got := tt.input.Icon()
			if got != tt.want {
				t.Errorf("InputType.Icon() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectionResult_SuggestedName(t *testing.T) {
	// Test that GitHub refs generate appropriate suggested names
	prRef := &github.ParsedGitHubRef{
		Type:     github.RefTypePR,
		Owner:    "owner",
		Repo:     "myrepo",
		PRNumber: 123,
	}
	if name := prRef.SuggestedSessionName(); name != "pr-123-myrepo" {
		t.Errorf("PR suggested name = %q, want %q", name, "pr-123-myrepo")
	}

	branchRef := &github.ParsedGitHubRef{
		Type:   github.RefTypeBranch,
		Owner:  "owner",
		Repo:   "myrepo",
		Branch: "feature/test",
	}
	if name := branchRef.SuggestedSessionName(); name != "myrepo-feature-test" {
		t.Errorf("Branch suggested name = %q, want %q", name, "myrepo-feature-test")
	}

	repoRef := &github.ParsedGitHubRef{
		Type:  github.RefTypeRepo,
		Owner: "owner",
		Repo:  "myrepo",
	}
	if name := repoRef.SuggestedSessionName(); name != "myrepo" {
		t.Errorf("Repo suggested name = %q, want %q", name, "myrepo")
	}
}
