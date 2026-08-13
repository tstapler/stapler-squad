package session

import (
	"testing"

	"github.com/tstapler/stapler-squad/github"
	"github.com/zalando/go-keyring"
)

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

// stapler-squad: registering a GitHub Enterprise domain must make the omnibar
// able to parse URLs from it, e.g. https://github.example-corp.com/engineering/widget-service/pull/370
func TestParseGitHubURLWithHosts_RecognizesEnterprisePRURL(t *testing.T) {
	hosts := []string{"github.example-corp.com"}
	ref, err := ParseGitHubURLWithHosts("https://github.example-corp.com/engineering/widget-service/pull/370", hosts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.Host != "github.example-corp.com" || ref.Owner != "engineering" || ref.Repo != "widget-service" || ref.PRNumber != 370 || ref.Type != GitHubRefTypePR {
		t.Errorf("got %+v, want Host=github.example-corp.com Owner=engineering Repo=widget-service PRNumber=370 Type=PR", ref)
	}
}

// TestParseGitHubURLWithHosts_MatchesRegardlessOfHostCase is the regression
// test for the round-2 review finding: GHE hosts are free-text admin/user
// input (unlike the hardcoded "github.com"), so a registered host typed in
// one case must still match a URL pasted in another case.
func TestParseGitHubURLWithHosts_MatchesRegardlessOfHostCase(t *testing.T) {
	hosts := []string{"Github.Example-Corp.com"}
	ref, err := ParseGitHubURLWithHosts("https://GITHUB.EXAMPLE-CORP.COM/engineering/widget-service/pull/370", hosts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.Host != "github.example-corp.com" || ref.Owner != "engineering" || ref.Repo != "widget-service" || ref.PRNumber != 370 {
		t.Errorf("got %+v, want Host=github.example-corp.com Owner=engineering Repo=widget-service PRNumber=370", ref)
	}
}

func TestParseGitHubURLWithHosts_RejectsUnregisteredHost(t *testing.T) {
	// Without the host registered, an enterprise URL must not be silently
	// mis-parsed against github.com.
	_, err := ParseGitHubURLWithHosts("https://github.example-corp.com/engineering/widget-service/pull/370", nil)
	if err == nil {
		t.Errorf("expected error for unregistered enterprise host, got nil")
	}
}

func TestParseGitHubURLWithHosts_RejectsPathTraversalSegments(t *testing.T) {
	hosts := []string{"github.example-corp.com"}
	cases := []string{
		"https://github.example-corp.com/../..",
		"https://github.example-corp.com/owner/../repo/pull/1",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			ref, err := ParseGitHubURLWithHosts(in, hosts)
			if err == nil {
				t.Errorf("ParseGitHubURLWithHosts(%q) = %+v, want error (traversal segment)", in, ref)
			}
		})
	}
}

func TestIsGitHubURLWithHosts(t *testing.T) {
	hosts := []string{"github.example-corp.com"}
	if !IsGitHubURLWithHosts("https://github.example-corp.com/engineering/widget-service/pull/370", hosts) {
		t.Errorf("expected true for registered enterprise host URL")
	}
	if IsGitHubURLWithHosts("https://github.example-corp.com/engineering/widget-service/pull/370", nil) {
		t.Errorf("expected false for unregistered enterprise host URL")
	}
}

func TestGetRepoPath_UsesHostSpecificSubdirectory(t *testing.T) {
	m := NewRepoPathManagerWithBase("/tmp/repos-base")
	path := m.GetRepoPath(&GitHubRef{Host: "github.example-corp.com", Owner: "engineering", Repo: "widget-service"})
	want := "/tmp/repos-base/github.example-corp.com/engineering/widget-service"
	if path != want {
		t.Errorf("GetRepoPath() = %q, want %q", path, want)
	}
}

func TestGetCloneURL_UsesHostSpecificURL(t *testing.T) {
	m := NewRepoPathManagerWithBase("/tmp/repos-base")
	url := m.GetCloneURL(&GitHubRef{Host: "github.example-corp.com", Owner: "engineering", Repo: "widget-service"})
	want := "https://github.example-corp.com/engineering/widget-service.git"
	if url != want {
		t.Errorf("GetCloneURL() = %q, want %q", url, want)
	}
}

// TestGetCloneURL_EmbedsKeychainToken_When_HostHasStoredAccount locks in the
// token-injection branch of GetCloneURL, which previously had no test
// coverage — the exact code path that EnsureRepoCloned's credential-logging
// fix (host/owner/repo-only logging, post-clone `git remote set-url`
// stripping, sanitizeCloneOutput) depends on actually firing.
func TestGetCloneURL_EmbedsKeychainToken_When_HostHasStoredAccount(t *testing.T) {
	keyring.MockInit()
	host := "github.example-corp.com"
	if err := github.SetKeychainTokenForAccount(host, "octocat", "token-abc123"); err != nil {
		t.Fatalf("SetKeychainTokenForAccount failed: %v", err)
	}

	m := NewRepoPathManagerWithBase("/tmp/repos-base")
	url := m.GetCloneURL(&GitHubRef{Host: host, Owner: "engineering", Repo: "widget-service"})
	want := "https://x-access-token:token-abc123@github.example-corp.com/engineering/widget-service.git"
	if url != want {
		t.Errorf("GetCloneURL() = %q, want %q", url, want)
	}
}

// TestParseGitHubURLWithHosts_RejectsLocalLookingPaths is the regression test
// for the dropped path-vs-shorthand guard: the "owner/repo" shorthand regex
// can't distinguish a real shorthand from a relative/absolute/home-relative
// local path, since both are just "segment/segment". Concretely,
// ParseGitHubURL(".git/repo") used to be rejected (see the pre-refactor
// implementation's `!strings.HasPrefix(input, ".")` guard) but started
// succeeding with Owner=".git" once shorthand parsing moved into the shared
// github package, which has no equivalent guard.
func TestParseGitHubURLWithHosts_RejectsLocalLookingPaths(t *testing.T) {
	cases := []string{
		".git/repo",
		"./relative/path",
		"/absolute/path",
		"~/home/path",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			ref, err := ParseGitHubURL(in)
			if err == nil {
				t.Errorf("ParseGitHubURL(%q) = %+v, want error (local-looking path)", in, ref)
			}
		})
	}
}

// TestGitHubRefPRURL_should_BuildCanonicalURL_When_PRNumberSet is the
// regression test for the PR #463 bug: CreateSession parsed a GitHub PR URL
// into a GitHubRef but never populated GitHubPRURL on the created session,
// leaving the VCS panel's PR link broken. PRURL() is the single function both
// CreateSession (server/services/session_service.go) and the startup backfill
// migration (github_pr_url_backfill.go) now call instead of formatting the
// URL inline in two places.
func TestGitHubRefPRURL_should_BuildCanonicalURL_When_PRNumberSet(t *testing.T) {
	ref := &GitHubRef{Host: "github.com", Owner: "tstapler", Repo: "stapler-squad", PRNumber: 463}

	got := ref.PRURL()

	want := "https://github.com/tstapler/stapler-squad/pull/463"
	if got != want {
		t.Errorf("PRURL() = %q, want %q", got, want)
	}
}

// TestGitHubRefPRURL_should_NormalizeHost_When_HostIsGitHubEnterprise proves
// PRURL() routes through github.NormalizeHost rather than embedding the raw
// host string, so GHES hosts are handled the same way whether the ref came
// from CreateSession's live parse or the backfill's path-recovered host.
func TestGitHubRefPRURL_should_NormalizeHost_When_HostIsGitHubEnterprise(t *testing.T) {
	ref := &GitHubRef{Host: "github.mycorp.com", Owner: "acme", Repo: "widgets", PRNumber: 7}

	got := ref.PRURL()

	want := "https://" + github.NormalizeHost("github.mycorp.com") + "/acme/widgets/pull/7"
	if got != want {
		t.Errorf("PRURL() = %q, want %q", got, want)
	}
}

// TestGitHubRefPRURL_should_ReturnEmpty_When_PRNumberNotSet guards the
// zero-value case (a repo/branch ref, not a PR ref) — must not fabricate a
// "/pull/0" URL.
func TestGitHubRefPRURL_should_ReturnEmpty_When_PRNumberNotSet(t *testing.T) {
	ref := &GitHubRef{Host: "github.com", Owner: "tstapler", Repo: "stapler-squad"}

	if got := ref.PRURL(); got != "" {
		t.Errorf("PRURL() = %q, want empty string when PRNumber is unset", got)
	}
}

// TestGitHubRefPRURL_should_ReturnEmpty_When_RefIsNil guards the nil-receiver
// case — CreateSession's gitHubRef is a *GitHubRef that can be nil for
// non-GitHub session creation, so PRURL() must not panic if ever called on it.
func TestGitHubRefPRURL_should_ReturnEmpty_When_RefIsNil(t *testing.T) {
	var ref *GitHubRef

	if got := ref.PRURL(); got != "" {
		t.Errorf("PRURL() = %q, want empty string for a nil ref", got)
	}
}
