package session

import "testing"

func TestGitHubMetadataView_IsPRSession(t *testing.T) {
	tests := []struct {
		name     string
		gh       GitHubMetadataView
		expected bool
	}{
		{"PR number > 0", GitHubMetadataView{PRNumber: 42}, true},
		{"PR number == 0", GitHubMetadataView{PRNumber: 0}, false},
		{"empty", GitHubMetadataView{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.gh.IsPRSession(); got != tt.expected {
				t.Errorf("IsPRSession() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGitHubMetadataView_RepoFullName(t *testing.T) {
	tests := []struct {
		name     string
		gh       GitHubMetadataView
		expected string
	}{
		{"both set", GitHubMetadataView{Owner: "octocat", Repo: "hello-world"}, "octocat/hello-world"},
		{"owner missing", GitHubMetadataView{Repo: "hello-world"}, ""},
		{"repo missing", GitHubMetadataView{Owner: "octocat"}, ""},
		{"both missing", GitHubMetadataView{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.gh.RepoFullName(); got != tt.expected {
				t.Errorf("RepoFullName() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestGitHubMetadataView_PRDisplayInfo(t *testing.T) {
	tests := []struct {
		name     string
		gh       GitHubMetadataView
		expected string
	}{
		{
			"PR session",
			GitHubMetadataView{PRNumber: 42, Owner: "octocat", Repo: "hello-world"},
			"PR #42 on octocat/hello-world",
		},
		{
			"not a PR session",
			GitHubMetadataView{Owner: "octocat", Repo: "hello-world"},
			"",
		},
		{
			"empty",
			GitHubMetadataView{},
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.gh.PRDisplayInfo(); got != tt.expected {
				t.Errorf("PRDisplayInfo() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestGitHubMetadataView_IsGitHubSession(t *testing.T) {
	tests := []struct {
		name     string
		gh       GitHubMetadataView
		expected bool
	}{
		{"both set", GitHubMetadataView{Owner: "octocat", Repo: "hello-world"}, true},
		{"owner only", GitHubMetadataView{Owner: "octocat"}, false},
		{"repo only", GitHubMetadataView{Repo: "hello-world"}, false},
		{"empty", GitHubMetadataView{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.gh.IsGitHubSession(); got != tt.expected {
				t.Errorf("IsGitHubSession() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGitHubMetadataView_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		gh       GitHubMetadataView
		expected bool
	}{
		{"all empty", GitHubMetadataView{}, true},
		{"PR number set", GitHubMetadataView{PRNumber: 1}, false},
		{"URL set", GitHubMetadataView{PRURL: "https://github.com/x/y/pull/1"}, false},
		{"owner set", GitHubMetadataView{Owner: "octocat"}, false},
		{"repo set", GitHubMetadataView{Repo: "hello-world"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.gh.IsEmpty(); got != tt.expected {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.expected)
			}
		})
	}
}
