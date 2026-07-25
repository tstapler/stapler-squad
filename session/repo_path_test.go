package session

import "testing"

// stapler-squad#152 security review: GitHub owner/repo regexes only exclude
// "/" from the captured segments, so "." and ".." pass through syntactically.
// GetRepoPath joins Owner/Repo directly into a filesystem path, so a crafted
// input like "https://github.com/../.." must be rejected before it ever
// reaches EnsureRepoCloned, not allowed to resolve outside the intended
// ~/.stapler-squad/repos/github.com/ subtree.
func TestParseGitHubURL_RejectsPathTraversalSegments(t *testing.T) {
	cases := []string{
		"https://github.com/../..",
		"https://github.com/../../etc",
		"https://github.com/owner/..",
		"https://github.com/../repo",
		"https://github.com/owner/../repo/tree/main",
		"https://github.com/owner/../repo/pull/1",
		"owner/..",
		"../repo",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			ref, err := ParseGitHubURL(in)
			if err == nil {
				t.Errorf("ParseGitHubURL(%q) = %+v, want error (traversal segment)", in, ref)
			}
		})
	}
}

func TestParseGitHubURL_HappyPath(t *testing.T) {
	ref, err := ParseGitHubURL("https://github.com/owner/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.Owner != "owner" || ref.Repo != "repo" {
		t.Errorf("got Owner=%q Repo=%q, want Owner=owner Repo=repo", ref.Owner, ref.Repo)
	}
}

func TestParseGitHubURL_ShorthandHappyPath(t *testing.T) {
	ref, err := ParseGitHubURL("owner/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.Owner != "owner" || ref.Repo != "repo" {
		t.Errorf("got Owner=%q Repo=%q, want Owner=owner Repo=repo", ref.Owner, ref.Repo)
	}
}

func TestGetRepoPath_StaysWithinBaseDir(t *testing.T) {
	m := NewRepoPathManagerWithBase("/tmp/repos-base")
	path := m.GetRepoPath(&GitHubRef{Owner: "owner", Repo: "repo"})
	want := "/tmp/repos-base/github.com/owner/repo"
	if path != want {
		t.Errorf("GetRepoPath() = %q, want %q", path, want)
	}
}
