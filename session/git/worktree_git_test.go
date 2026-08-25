package git

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/tmux"
)

// TestPrNumberFromURLRe_ExtractsTrailingNumber is the regression test for
// CreatePR's number-resolution fix: prNumber must be derived from the PR URL
// (a plain string operation) rather than solely from a second `gh pr view`
// subprocess call whose failure was previously silently swallowed, leaving
// prNumber at 0 even though prURL had already resolved correctly — which then
// made EnablePRAutoMerge fail with "no pull requests found" for a PR that
// otherwise pushed and tracked fine. See CreatePR in worktree_git.go.
func TestPrNumberFromURLRe_ExtractsTrailingNumber(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		url  string
		want string // "" means no match
	}{
		{"plain URL", "https://github.com/tstapler/stapler-squad/pull/172", "172"},
		{"trailing slash", "https://github.com/tstapler/stapler-squad/pull/172/", "172"},
		{"single digit", "https://github.com/owner/repo/pull/9", "9"},
		{"not a PR URL", "https://github.com/tstapler/stapler-squad/issues/172", ""},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := prNumberFromURLRe.FindStringSubmatch(tt.url)
			if tt.want == "" {
				if m != nil {
					t.Errorf("FindStringSubmatch(%q) = %v; want no match", tt.url, m)
				}
				return
			}
			if m == nil {
				t.Fatalf("FindStringSubmatch(%q) = nil; want match with group %q", tt.url, tt.want)
			}
			if m[1] != tt.want {
				t.Errorf("FindStringSubmatch(%q) group = %q; want %q", tt.url, m[1], tt.want)
			}
		})
	}
}

// TestIsDirtyWithHint_ReturnsLocallyComputedValue_WhenCacheIsWrittenByRacingGoroutine
// verifies the return-own-observation invariant: IsDirtyWithHint must return the
// locally-computed value, not a re-read of the cache slot.
//
// With atomic.Value the write is unconditional, so a race can't suppress our Store.
// The test simulates a racing goroutine that stores false into the cache WHILE our
// git subprocess is running; our code must still return true (its own observation).
//
// Uses a gitSpyCommandRunner (via WithCommandRunner) rather than the
// executor.Executor-based mock this test used before runGitCommand was migrated
// onto CommandRunner unconditionally (see ADR-002's addendum) — spy.runFunc plays
// the same role raceSimulatorExecutor's raceSetup hook used to.
func TestIsDirtyWithHint_ReturnsLocallyComputedValue_WhenCacheIsWrittenByRacingGoroutine(t *testing.T) {
	t.Parallel()
	spy := &gitSpyCommandRunner{}
	g := NewGitWorktreeFromStorageWithExecutor(
		"/fake/repo", "/fake/worktree", "test-session", "test-branch", "",
		WithCommandRunner(spy),
	)

	// runFunc runs inside Run, simulating a concurrent goroutine that stores
	// dirty=false while our git call is "in flight".
	spy.runFunc = func() ([]byte, error) {
		g.isDirtyCache.Store(dirtyCacheState{dirty: false, time: time.Now()})
		return []byte("M file.txt\n"), nil // our goroutine sees the worktree as dirty
	}

	// Start with an invalid cache so IsDirtyWithHint takes the slow (git) path.
	g.isDirtyCache.Store(dirtyCacheState{}) // zero time = cache invalid

	got, err := g.IsDirtyWithHint(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// We computed dirty=true. The racing goroutine stored false. The invariant:
	// we must return our own observation (true), overwriting the racing store.
	if !got {
		t.Errorf("IsDirtyWithHint = false; want true (locally computed value)")
	}
}

// TestIsDirtyWithHint_BacksOffAfterError proves that a failing `git status` (e.g. the
// worktree directory is missing — the stale-path-after-rework bug) is cached with a
// backoff TTL rather than re-run on every call: a second call made immediately after a
// failure must return the same error without spawning another subprocess.
//
// Uses a gitSpyCommandRunner (via WithCommandRunner) rather than the
// executor.Executor-based countingErrExecutor mock this test used before
// runGitCommand was migrated onto CommandRunner unconditionally (see ADR-002's
// addendum) — len(spy.runCalls) plays the same role countingErrExecutor.calls used to.
func TestIsDirtyWithHint_BacksOffAfterError(t *testing.T) {
	t.Parallel()
	spy := &gitSpyCommandRunner{
		runOut: []byte("fatal: cannot change to '/fake/worktree': No such file or directory"),
		runErr: exec.ErrNotFound,
	}
	g := NewGitWorktreeFromStorageWithExecutor(
		"/fake/repo", "/fake/worktree", "test-session", "test-branch", "",
		WithCommandRunner(spy),
	)
	g.isDirtyCache.Store(dirtyCacheState{}) // zero time = cache invalid

	if _, err := g.IsDirtyWithHint(false); err == nil {
		t.Fatalf("IsDirtyWithHint() error = nil; want an error from the failing git command")
	}
	if len(spy.runCalls) != 1 {
		t.Fatalf("calls after first (failing) check = %d; want 1", len(spy.runCalls))
	}

	if _, err := g.IsDirtyWithHint(false); err == nil {
		t.Fatalf("second IsDirtyWithHint() error = nil; want the cached error")
	}
	if len(spy.runCalls) != 1 {
		t.Errorf("calls after second check within backoff TTL = %d; want still 1 (no new subprocess spawned)", len(spy.runCalls))
	}
}

// TestParsePRStatusPayload_ConflictDetection is a table-driven test over the
// documented mergeable/mergeStateStatus enum combinations (plan.md Story 1.3.1),
// proving the HasConflicts OR condition is correct for both the trigger cases
// and the near-miss cases that must NOT trigger.
func TestParsePRStatusPayload_ConflictDetection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		mergeable        string
		mergeStateStatus string
		wantHasConflicts bool
	}{
		{
			name:             "MERGEABLE/CLEAN is healthy, no conflict",
			mergeable:        "MERGEABLE",
			mergeStateStatus: "CLEAN",
			wantHasConflicts: false,
		},
		{
			name:             "CONFLICTING/DIRTY is a conflict",
			mergeable:        "CONFLICTING",
			mergeStateStatus: "DIRTY",
			wantHasConflicts: true,
		},
		{
			name:             "CONFLICTING/BLOCKED is a conflict (mergeable is authoritative)",
			mergeable:        "CONFLICTING",
			mergeStateStatus: "BLOCKED",
			wantHasConflicts: true,
		},
		{
			name:             "UNKNOWN/UNKNOWN is transient, no signal",
			mergeable:        "UNKNOWN",
			mergeStateStatus: "UNKNOWN",
			wantHasConflicts: false,
		},
		{
			name:             "MERGEABLE/BLOCKED is a review/check gate, not a conflict",
			mergeable:        "MERGEABLE",
			mergeStateStatus: "BLOCKED",
			wantHasConflicts: false,
		},
		{
			name:             "MERGEABLE/BEHIND is behind base, not a conflict",
			mergeable:        "MERGEABLE",
			mergeStateStatus: "BEHIND",
			wantHasConflicts: false,
		},
		{
			name:             "MERGEABLE/DIRTY is a conflict (cli/cli#9583 stale-mergeable scenario)",
			mergeable:        "MERGEABLE",
			mergeStateStatus: "DIRTY",
			wantHasConflicts: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := []byte(`{"statusCheckRollup":[],"reviews":[],"comments":[],"mergeable":"` +
				tt.mergeable + `","mergeStateStatus":"` + tt.mergeStateStatus + `"}`)

			status, err := parsePRStatusPayload(raw)
			if err != nil {
				t.Fatalf("parsePRStatusPayload() error = %v", err)
			}

			if status.HasConflicts != tt.wantHasConflicts {
				t.Errorf("HasConflicts = %v; want %v", status.HasConflicts, tt.wantHasConflicts)
			}

			if tt.wantHasConflicts {
				if !strings.Contains(status.FeedbackText, "## Merge conflict") {
					t.Errorf("FeedbackText = %q; want it to contain %q", status.FeedbackText, "## Merge conflict")
				}
			} else {
				if strings.Contains(status.FeedbackText, "## Merge conflict") {
					t.Errorf("FeedbackText = %q; want no %q section", status.FeedbackText, "## Merge conflict")
				}
			}
		})
	}
}

// TestParsePRStatusPayload_ConflictGuidanceText proves Story 1.2.1's guidance
// text (--force-with-lease, .gitignore suspicion, leave-markers-and-stop,
// mandatory git diff --stat) renders into FeedbackText's conflict section, and
// that it renders identically regardless of which field (mergeable vs.
// mergeStateStatus) tripped the HasConflicts OR condition.
func TestParsePRStatusPayload_ConflictGuidanceText(t *testing.T) {
	t.Parallel()
	wantSubstrings := []string{
		"--force-with-lease",
		".gitignore",
		"leave the conflict markers",
		"git diff --stat",
	}

	t.Run("CONFLICTING/DIRTY renders guidance text", func(t *testing.T) {
		raw := []byte(`{"statusCheckRollup":[],"reviews":[],"comments":[],"mergeable":"CONFLICTING","mergeStateStatus":"DIRTY"}`)

		status, err := parsePRStatusPayload(raw)
		if err != nil {
			t.Fatalf("parsePRStatusPayload() error = %v", err)
		}

		for _, want := range wantSubstrings {
			if !strings.Contains(status.FeedbackText, want) {
				t.Errorf("FeedbackText missing %q; got %q", want, status.FeedbackText)
			}
		}
	})

	t.Run("MERGEABLE/DIRTY (cli/cli#9583 stale-mergeable case) renders identical guidance text", func(t *testing.T) {
		raw := []byte(`{"statusCheckRollup":[],"reviews":[],"comments":[],"mergeable":"MERGEABLE","mergeStateStatus":"DIRTY"}`)

		status, err := parsePRStatusPayload(raw)
		if err != nil {
			t.Fatalf("parsePRStatusPayload() error = %v", err)
		}

		for _, want := range wantSubstrings {
			if !strings.Contains(status.FeedbackText, want) {
				t.Errorf("FeedbackText missing %q; got %q", want, status.FeedbackText)
			}
		}
	})
}

// TestParsePRStatusPayload_CIFailing covers the pre-existing, previously
// untested CIFailing detection logic: a terminal FAILURE conclusion must set
// CIFailing=true, while a non-terminal IN_PROGRESS check must not.
func TestParsePRStatusPayload_CIFailing(t *testing.T) {
	t.Parallel()
	t.Run("terminal FAILURE conclusion sets CIFailing true", func(t *testing.T) {
		raw := []byte(`{"statusCheckRollup":[{"name":"build","conclusion":"FAILURE"}],"reviews":[],"comments":[],"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN"}`)

		status, err := parsePRStatusPayload(raw)
		if err != nil {
			t.Fatalf("parsePRStatusPayload() error = %v", err)
		}

		if !status.CIFailing {
			t.Errorf("CIFailing = false; want true")
		}
		if !strings.Contains(status.FeedbackText, "## Failing CI checks") {
			t.Errorf("FeedbackText missing %q; got %q", "## Failing CI checks", status.FeedbackText)
		}
		if !strings.Contains(status.FeedbackText, "build FAILED") {
			t.Errorf("FeedbackText missing %q; got %q", "build FAILED", status.FeedbackText)
		}
	})

	t.Run("non-terminal IN_PROGRESS leaves CIFailing false", func(t *testing.T) {
		raw := []byte(`{"statusCheckRollup":[{"name":"build","status":"IN_PROGRESS"}],"reviews":[],"comments":[],"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN"}`)

		status, err := parsePRStatusPayload(raw)
		if err != nil {
			t.Fatalf("parsePRStatusPayload() error = %v", err)
		}

		if status.CIFailing {
			t.Errorf("CIFailing = true; want false")
		}
		if strings.Contains(status.FeedbackText, "## Failing CI checks") {
			t.Errorf("FeedbackText = %q; want no %q section", status.FeedbackText, "## Failing CI checks")
		}
	})
}

// TestParsePRStatusPayload_HasBlockingReviews covers the pre-existing,
// previously untested HasBlockingReviews detection logic: a CHANGES_REQUESTED
// review must set HasBlockingReviews=true, while an APPROVED-only review set
// must not.
func TestParsePRStatusPayload_HasBlockingReviews(t *testing.T) {
	t.Parallel()
	t.Run("CHANGES_REQUESTED review sets HasBlockingReviews true", func(t *testing.T) {
		raw := []byte(`{"statusCheckRollup":[],"reviews":[{"state":"CHANGES_REQUESTED","body":"Fix the null check","author":{"login":"reviewer1"}}],"comments":[],"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN"}`)

		status, err := parsePRStatusPayload(raw)
		if err != nil {
			t.Fatalf("parsePRStatusPayload() error = %v", err)
		}

		if !status.HasBlockingReviews {
			t.Errorf("HasBlockingReviews = false; want true")
		}
		if !strings.Contains(status.FeedbackText, "## Review: changes requested by @reviewer1") {
			t.Errorf("FeedbackText missing %q; got %q", "## Review: changes requested by @reviewer1", status.FeedbackText)
		}
		if !strings.Contains(status.FeedbackText, "Fix the null check") {
			t.Errorf("FeedbackText missing %q; got %q", "Fix the null check", status.FeedbackText)
		}
		if status.HasReviewFeedback {
			t.Errorf("HasReviewFeedback = true; want false — a CHANGES_REQUESTED review must never count toward the new signal, only COMMENTED")
		}
	})

	t.Run("APPROVED-only reviews leave HasBlockingReviews false", func(t *testing.T) {
		raw := []byte(`{"statusCheckRollup":[],"reviews":[{"state":"APPROVED","body":"LGTM","author":{"login":"reviewer1"}}],"comments":[],"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN"}`)

		status, err := parsePRStatusPayload(raw)
		if err != nil {
			t.Fatalf("parsePRStatusPayload() error = %v", err)
		}

		if status.HasBlockingReviews {
			t.Errorf("HasBlockingReviews = true; want false")
		}
		if strings.Contains(status.FeedbackText, "## Review:") {
			t.Errorf("FeedbackText = %q; want no %q section", status.FeedbackText, "## Review:")
		}
	})
}

// TestParsePRStatusPayload_ConflictSectionOrderedFirst proves the "conflict
// section is always ordered first" design decision (features.md §2A): when a
// PR has both HasConflicts and CIFailing true, the conflict section must
// precede the CI section in FeedbackText.
func TestParsePRStatusPayload_ConflictSectionOrderedFirst(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"statusCheckRollup":[{"name":"build","conclusion":"FAILURE"}],"reviews":[],"comments":[],"mergeable":"CONFLICTING","mergeStateStatus":"DIRTY"}`)

	status, err := parsePRStatusPayload(raw)
	if err != nil {
		t.Fatalf("parsePRStatusPayload() error = %v", err)
	}

	conflictIdx := strings.Index(status.FeedbackText, "## Merge conflict")
	ciIdx := strings.Index(status.FeedbackText, "## Failing CI checks")

	if conflictIdx == -1 {
		t.Fatalf("FeedbackText missing %q section; got %q", "## Merge conflict", status.FeedbackText)
	}
	if ciIdx == -1 {
		t.Fatalf("FeedbackText missing %q section; got %q", "## Failing CI checks", status.FeedbackText)
	}
	if conflictIdx >= ciIdx {
		t.Errorf("conflict section index %d not before CI section index %d in FeedbackText: %q", conflictIdx, ciIdx, status.FeedbackText)
	}
}

// TestIsSubstantiveFeedback proves the length-only noise filter used to keep
// bare "LGTM"-style feedback out of the HasReviewFeedback signal.
func TestIsSubstantiveFeedback(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"empty string", "", false},
		{"whitespace only", "   ", false},
		{"bare lgtm", "lgtm", false},
		{"bare nice", "nice", false},
		{"exactly 10 runes (boundary)", "1234567890", true},
		{"9 runes (one below threshold)", "123456789", false},
		{"substantive feedback", "Consider extracting this into a helper function.", true},
		{"10 multi-byte runes below byte-length would suggest", "一二三四五六七八九十", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSubstantiveFeedback(tt.body); got != tt.want {
				t.Errorf("isSubstantiveFeedback(%q) = %v; want %v", tt.body, got, tt.want)
			}
		})
	}
}

// TestIsExcludedBotAuthor proves automated bot accounts are excluded from
// the substantive-feedback signal except Copilot's own review account, which
// this feature exists to capture.
func TestIsExcludedBotAuthor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		login string
		want  bool
	}{
		{"human author", "tstapler", false},
		{"copilot review bot is exempted", "copilot-pull-request-reviewer[bot]", false},
		{"github-actions bot is excluded", "github-actions[bot]", true},
		{"codecov bot is excluded", "codecov[bot]", true},
		{"dependabot is excluded", "dependabot[bot]", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isExcludedBotAuthor(tt.login); got != tt.want {
				t.Errorf("isExcludedBotAuthor(%q) = %v; want %v", tt.login, got, tt.want)
			}
		})
	}
}

// TestParsePRStatusPayload_HasReviewFeedback_should_ExcludeNonCopilotBotAuthor
// is the regression test for pre-mortem.md #5: a long-enough recurring bot
// comment (coverage report, CI status summary) must never set
// HasReviewFeedback, even though its body alone would pass
// isSubstantiveFeedback — only a human or Copilot's own review account
// counts. The comment must still appear in FeedbackText (existing
// "include all comments" behavior preserved) without counting toward the
// signal.
func TestParsePRStatusPayload_HasReviewFeedback_should_ExcludeNonCopilotBotAuthor(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"statusCheckRollup":[],"reviews":[],"comments":[{"body":"Coverage decreased (-0.1%) to 95.3% on this pull request.","author":{"login":"codecov[bot]"},"createdAt":"2026-08-02T13:00:00Z"}],"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN"}`)

	status, err := parsePRStatusPayload(raw)
	if err != nil {
		t.Fatalf("parsePRStatusPayload() error = %v", err)
	}

	if status.HasReviewFeedback {
		t.Errorf("HasReviewFeedback = true; want false — a substantive comment from an excluded bot author must not count toward the signal")
	}
	if !status.LatestFeedbackAt.IsZero() {
		t.Errorf("LatestFeedbackAt = %v; want zero value", status.LatestFeedbackAt)
	}
	if !strings.Contains(status.FeedbackText, "@codecov[bot]: Coverage decreased") {
		t.Errorf("FeedbackText missing the bot comment (existing include-all-comments behavior); got %q", status.FeedbackText)
	}
}

// TestParsePRStatusPayload_HasReviewFeedback_should_ExcludeNonCopilotBotCommentedReview
// is the COMMENTED-review counterpart: a substantive COMMENTED review from a
// non-Copilot bot must never be captured into commentReviews or count toward
// HasReviewFeedback.
func TestParsePRStatusPayload_HasReviewFeedback_should_ExcludeNonCopilotBotCommentedReview(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"statusCheckRollup":[],"reviews":[{"state":"COMMENTED","body":"This PR increases bundle size by 12%.","author":{"login":"bundlesize-bot[bot]"},"submittedAt":"2026-08-02T14:00:00Z"}],"comments":[],"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN"}`)

	status, err := parsePRStatusPayload(raw)
	if err != nil {
		t.Fatalf("parsePRStatusPayload() error = %v", err)
	}

	if status.HasReviewFeedback {
		t.Errorf("HasReviewFeedback = true; want false")
	}
	if len(status.commentReviews) != 0 {
		t.Errorf("commentReviews = %v; want empty — a non-Copilot bot's COMMENTED review must not be captured", status.commentReviews)
	}
}

// TestParsePRStatusPayload_HasReviewFeedback_CommentedReview proves a
// substantive COMMENTED-state review (Copilot's typical review posture) sets
// HasReviewFeedback and captures the review's submittedAt as LatestFeedbackAt.
func TestParsePRStatusPayload_HasReviewFeedback_CommentedReview(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"statusCheckRollup":[],"reviews":[{"state":"COMMENTED","body":"Consider extracting this into a helper function.","author":{"login":"copilot-pull-request-reviewer[bot]"},"submittedAt":"2026-08-02T14:32:07Z"}],"comments":[],"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN"}`)

	status, err := parsePRStatusPayload(raw)
	if err != nil {
		t.Fatalf("parsePRStatusPayload() error = %v", err)
	}

	if !status.HasReviewFeedback {
		t.Errorf("HasReviewFeedback = false; want true")
	}
	want, _ := time.Parse(time.RFC3339, "2026-08-02T14:32:07Z")
	if !status.LatestFeedbackAt.Equal(want) {
		t.Errorf("LatestFeedbackAt = %v; want %v", status.LatestFeedbackAt, want)
	}
}

// TestParsePRStatusPayload_HasReviewFeedback_PlainComment proves a substantive
// plain PR comment (no review state at all) also sets HasReviewFeedback.
func TestParsePRStatusPayload_HasReviewFeedback_PlainComment(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"statusCheckRollup":[],"reviews":[],"comments":[{"body":"Please rebase onto main.","author":{"login":"tstapler"},"createdAt":"2026-08-02T13:00:00Z"}],"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN"}`)

	status, err := parsePRStatusPayload(raw)
	if err != nil {
		t.Fatalf("parsePRStatusPayload() error = %v", err)
	}

	if !status.HasReviewFeedback {
		t.Errorf("HasReviewFeedback = false; want true")
	}
	want, _ := time.Parse(time.RFC3339, "2026-08-02T13:00:00Z")
	if !status.LatestFeedbackAt.Equal(want) {
		t.Errorf("LatestFeedbackAt = %v; want %v", status.LatestFeedbackAt, want)
	}
}

// TestParsePRStatusPayload_HasReviewFeedback_NonSubstantiveIgnored proves a
// bare "lgtm" COMMENTED review never sets HasReviewFeedback, while a bare
// "lgtm" plain comment still appears in generalComments/FeedbackText
// (existing "include all comments" behavior preserved) without counting
// toward the signal.
func TestParsePRStatusPayload_HasReviewFeedback_NonSubstantiveIgnored(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"statusCheckRollup":[],"reviews":[{"state":"COMMENTED","body":"lgtm","author":{"login":"copilot-pull-request-reviewer[bot]"},"submittedAt":"2026-08-02T14:00:00Z"}],"comments":[{"body":"lgtm","author":{"login":"tstapler"},"createdAt":"2026-08-02T13:00:00Z"}],"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN"}`)

	status, err := parsePRStatusPayload(raw)
	if err != nil {
		t.Fatalf("parsePRStatusPayload() error = %v", err)
	}

	if status.HasReviewFeedback {
		t.Errorf("HasReviewFeedback = true; want false")
	}
	if !status.LatestFeedbackAt.IsZero() {
		t.Errorf("LatestFeedbackAt = %v; want zero value", status.LatestFeedbackAt)
	}
	if len(status.commentReviews) != 0 {
		t.Errorf("commentReviews = %v; want empty — a non-substantive COMMENTED review must not be captured", status.commentReviews)
	}
	if !strings.Contains(status.FeedbackText, "@tstapler: lgtm") {
		t.Errorf("FeedbackText missing bare plain comment %q (existing include-all-comments behavior); got %q", "@tstapler: lgtm", status.FeedbackText)
	}
}

// TestParsePRStatusPayload_LatestFeedbackAt_should_ReturnMaxTimestamp_When_MultipleFeedbackItemsPresent
// proves LatestFeedbackAt is the max across both commentReviews and
// generalComments, not just one slice.
func TestParsePRStatusPayload_LatestFeedbackAt_should_ReturnMaxTimestamp_When_MultipleFeedbackItemsPresent(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"statusCheckRollup":[],"reviews":[{"state":"COMMENTED","body":"Consider extracting this into a helper function.","author":{"login":"copilot-pull-request-reviewer[bot]"},"submittedAt":"2026-08-02T14:32:07Z"}],"comments":[{"body":"Please rebase.","author":{"login":"tstapler"},"createdAt":"2026-08-02T15:10:00Z"}],"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN"}`)

	status, err := parsePRStatusPayload(raw)
	if err != nil {
		t.Fatalf("parsePRStatusPayload() error = %v", err)
	}

	if !status.HasReviewFeedback {
		t.Errorf("HasReviewFeedback = false; want true")
	}
	want, _ := time.Parse(time.RFC3339, "2026-08-02T15:10:00Z")
	if !status.LatestFeedbackAt.Equal(want) {
		t.Errorf("LatestFeedbackAt = %v; want %v (the later of the two feedback items)", status.LatestFeedbackAt, want)
	}
}

// TestParsePRStatusPayload_LatestFeedbackAt_should_ReturnZeroValue_When_NoSubstantiveFeedback
// proves HasReviewFeedback/LatestFeedbackAt stay at their zero values when
// nothing substantive was captured.
func TestParsePRStatusPayload_LatestFeedbackAt_should_ReturnZeroValue_When_NoSubstantiveFeedback(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"statusCheckRollup":[],"reviews":[],"comments":[{"body":"lgtm","author":{"login":"tstapler"},"createdAt":"2026-08-02T13:00:00Z"}],"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN"}`)

	status, err := parsePRStatusPayload(raw)
	if err != nil {
		t.Fatalf("parsePRStatusPayload() error = %v", err)
	}

	if status.HasReviewFeedback {
		t.Errorf("HasReviewFeedback = true; want false")
	}
	if !status.LatestFeedbackAt.IsZero() {
		t.Errorf("LatestFeedbackAt = %v; want zero value", status.LatestFeedbackAt)
	}
}

// TestParsePRStatusPayload_ReviewerCommentsSectionRendered proves render()
// emits a "## Reviewer comments" section for commentReviews, positioned after
// the "## Review: changes requested" block(s) and before "## PR comments".
func TestParsePRStatusPayload_ReviewerCommentsSectionRendered(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"statusCheckRollup":[],"reviews":[{"state":"CHANGES_REQUESTED","body":"Fix the null check","author":{"login":"reviewer1"}},{"state":"COMMENTED","body":"Consider extracting this into a helper function.","author":{"login":"copilot-pull-request-reviewer[bot]"},"submittedAt":"2026-08-02T14:32:07Z"}],"comments":[{"body":"Please rebase.","author":{"login":"tstapler"},"createdAt":"2026-08-02T15:10:00Z"}],"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN"}`)

	status, err := parsePRStatusPayload(raw)
	if err != nil {
		t.Fatalf("parsePRStatusPayload() error = %v", err)
	}

	if !strings.Contains(status.FeedbackText, "## Reviewer comments\n@copilot-pull-request-reviewer[bot]: Consider extracting this into a helper function.\n\n") {
		t.Errorf("FeedbackText missing expected Reviewer comments section; got %q", status.FeedbackText)
	}

	reviewIdx := strings.Index(status.FeedbackText, "## Review: changes requested")
	reviewerCommentsIdx := strings.Index(status.FeedbackText, "## Reviewer comments")
	prCommentsIdx := strings.Index(status.FeedbackText, "## PR comments")
	if reviewIdx == -1 || reviewerCommentsIdx == -1 || prCommentsIdx == -1 {
		t.Fatalf("expected all three sections present; got %q", status.FeedbackText)
	}
	if reviewIdx >= reviewerCommentsIdx || reviewerCommentsIdx >= prCommentsIdx {
		t.Errorf("section order wrong: review=%d, reviewerComments=%d, prComments=%d; want review < reviewerComments < prComments", reviewIdx, reviewerCommentsIdx, prCommentsIdx)
	}
}

// capturingGHRunner is a tmux.CommandRunner spy for CreatePR tests: it
// records every command's args and dispatches a canned response based on the
// gh subcommand (`pr list` for findExistingPR's pre-check, `pr create` for
// the actual creation), so tests can assert on the exact args CreatePR built
// without touching a real `gh` process. Mirrors gitSpyCommandRunner's shape,
// but branches on args the way capturingGHExecutor (removed once CreatePR's
// last executor.Executor-gated branch migrated onto commandRunner()) used to.
type capturingGHRunner struct {
	createArgs []string // captured args of the `gh pr create` invocation
	createOut  string   // output returned for `gh pr create`
}

func (r *capturingGHRunner) Run(_ context.Context, _ string, name string, args ...string) ([]byte, error) {
	if name == "gh" && len(args) > 0 && args[0] == "pr" && len(args) > 1 && args[1] == "list" {
		// findExistingPR's pre-check: report no existing PR so CreatePR
		// proceeds to `gh pr create`.
		return nil, exec.ErrNotFound
	}
	r.createArgs = append([]string(nil), args...)
	out := r.createOut
	if out == "" {
		out = "https://github.com/tstapler/stapler-squad/pull/172\n"
	}
	return []byte(out), nil
}

func (r *capturingGHRunner) Start(context.Context, string, string, ...string) (io.WriteCloser, io.ReadCloser, func() error, error) {
	return nil, nil, nil, fmt.Errorf("capturingGHRunner.Start not implemented")
}

func (r *capturingGHRunner) IsRemote() bool { return false }

var _ tmux.CommandRunner = (*capturingGHRunner)(nil)

// TestGitWorktree_CreatePR_PassesBaseBranch_When_NonEmpty proves Task 1.1.1a:
// a non-empty baseBranch is forwarded to `gh pr create` as `--base <value>`,
// closing the AC3 gap where the modal's base-branch field would otherwise be
// UI-only and silently ignored.
func TestGitWorktree_CreatePR_PassesBaseBranch_When_NonEmpty(t *testing.T) {
	t.Parallel()
	mock := &capturingGHRunner{}
	g := NewGitWorktreeFromStorageWithExecutor(
		"/fake/repo", "/fake/worktree", "test-session", "feature/rate-limit-toggle", "",
		WithCommandRunner(mock),
	)

	_, _, err := g.CreatePR(PRCreateOptions{Title: "Add rate limit toggle", Body: "Adds a per-user rate limit toggle.", BaseBranch: "release/1.2"})
	if err != nil {
		t.Fatalf("CreatePR() error = %v", err)
	}

	if mock.createArgs == nil {
		t.Fatalf("gh pr create was never invoked")
	}
	found := false
	for i, arg := range mock.createArgs {
		if arg == "--base" && i+1 < len(mock.createArgs) && mock.createArgs[i+1] == "release/1.2" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("gh pr create args = %v; want to contain %q %q", mock.createArgs, "--base", "release/1.2")
	}
}

// TestGitWorktree_CreatePR_OmitsBaseFlag_When_Empty is the regression guard
// for Task 1.1.1c's backward-compat requirement: an empty baseBranch must
// never append `--base`, preserving gh's own default-branch resolution for
// every pre-existing caller (e.g. the backlog automation path).
func TestGitWorktree_CreatePR_OmitsBaseFlag_When_Empty(t *testing.T) {
	t.Parallel()
	mock := &capturingGHRunner{}
	g := NewGitWorktreeFromStorageWithExecutor(
		"/fake/repo", "/fake/worktree", "test-session", "feature/rate-limit-toggle", "",
		WithCommandRunner(mock),
	)

	_, _, err := g.CreatePR(PRCreateOptions{Title: "Add rate limit toggle", Body: "Adds a per-user rate limit toggle."})
	if err != nil {
		t.Fatalf("CreatePR() error = %v", err)
	}

	if mock.createArgs == nil {
		t.Fatalf("gh pr create was never invoked")
	}
	for _, arg := range mock.createArgs {
		if arg == "--base" {
			t.Errorf("gh pr create args = %v; want no %q flag when baseBranch is empty", mock.createArgs, "--base")
		}
	}
}

// TestParsePRStatusPayload_Render_should_ProduceByteIdenticalOutput_When_GeneralCommentsRetyped
// proves the generalComments []string -> []prFeedbackItem retype alone
// introduced zero rendering drift: the "## PR comments" block is byte-for-byte
// identical to what the pre-retype append-time-constructed string produced.
func TestParsePRStatusPayload_Render_should_ProduceByteIdenticalOutput_When_GeneralCommentsRetyped(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"statusCheckRollup":[],"reviews":[],"comments":[{"body":"Please rebase.","author":{"login":"tstapler"},"createdAt":"2026-08-02T15:10:00Z"},{"body":"lgtm","author":{"login":"reviewer2"},"createdAt":"2026-08-02T15:11:00Z"}],"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN"}`)

	status, err := parsePRStatusPayload(raw)
	if err != nil {
		t.Fatalf("parsePRStatusPayload() error = %v", err)
	}

	want := "## PR comments\n@tstapler: Please rebase.\n\n@reviewer2: lgtm\n\n"
	if !strings.Contains(status.FeedbackText, want) {
		t.Errorf("FeedbackText missing byte-identical PR comments block; got %q, want substring %q", status.FeedbackText, want)
	}
}

// gitSpyRunCall records one tmux.CommandRunner.Run invocation.
type gitSpyRunCall struct {
	dir  string
	name string
	args []string
}

// gitSpyCommandRunner is a test tmux.CommandRunner spy: it records every Run
// call and returns a scripted response. Used to prove a CommandRunner
// injected via WithCommandRunner is actually consulted by GitWorktree's
// methods, not merely stored on the struct.
type gitSpyCommandRunner struct {
	runCalls []gitSpyRunCall
	runOut   []byte
	runErr   error
	// runFunc, when set, is invoked at the moment each Run call would have
	// executed the subprocess, in place of returning runOut/runErr directly.
	// Lets a test inject a side effect exactly when the "subprocess" runs
	// (e.g. simulating a concurrent goroutine racing the cache, the way
	// raceSimulatorExecutor's raceSetup hook used to), not just a canned
	// return value.
	runFunc func() ([]byte, error)
}

func (s *gitSpyCommandRunner) Run(_ context.Context, dir, name string, args ...string) ([]byte, error) {
	s.runCalls = append(s.runCalls, gitSpyRunCall{dir: dir, name: name, args: append([]string(nil), args...)})
	if s.runFunc != nil {
		return s.runFunc()
	}
	return s.runOut, s.runErr
}

func (s *gitSpyCommandRunner) Start(context.Context, string, string, ...string) (io.WriteCloser, io.ReadCloser, func() error, error) {
	return nil, nil, nil, fmt.Errorf("gitSpyCommandRunner.Start not implemented")
}

func (s *gitSpyCommandRunner) IsRemote() bool { return false }

var _ tmux.CommandRunner = (*gitSpyCommandRunner)(nil)

// TestWithCommandRunner_InjectedRunnerIsActuallyUsed is VIOLATION 1's required
// regression guard for GitWorktree: constructs one via
// NewGitWorktreeFromStorageWithExecutor with WithCommandRunner(spy), calls
// PushBranch() (a call site with no cmdExec dependency at all -- see
// worktree_git.go), and asserts the spy -- not tmux.LocalRunner -- actually
// received the "git push -u origin <branch>" call, scoped to the worktree
// path. Proves injection is wired all the way through to a real call site,
// not just stored on the struct.
func TestWithCommandRunner_InjectedRunnerIsActuallyUsed(t *testing.T) {
	spy := &gitSpyCommandRunner{}
	g := NewGitWorktreeFromStorageWithExecutor(
		"/fake/repo", "/fake/worktree", "test-session", "test-branch", "",
		WithCommandRunner(spy),
	)
	if g == nil {
		t.Fatal("NewGitWorktreeFromStorageWithExecutor returned nil")
	}

	if err := g.PushBranch(); err != nil {
		t.Fatalf("PushBranch returned error: %v", err)
	}

	if len(spy.runCalls) != 1 {
		t.Fatalf("spy.Run call count = %d, want 1 (PushBranch should route through the injected CommandRunner)", len(spy.runCalls))
	}
	call := spy.runCalls[0]
	wantArgs := []string{"push", "-u", "origin", "test-branch"}
	if call.dir != "/fake/worktree" || call.name != "git" || len(call.args) != len(wantArgs) {
		t.Fatalf("spy.Run call = %+v, want {dir: \"/fake/worktree\", name: \"git\", args: %v}", call, wantArgs)
	}
	for i, arg := range wantArgs {
		if call.args[i] != arg {
			t.Errorf("spy.Run args[%d] = %q, want %q", i, call.args[i], arg)
		}
	}
}

// TestRunGitCommand_UsesInjectedCommandRunner is the positive proof required
// by the FIX-FIRST re-review: runGitCommand (session/git/worktree_git.go),
// the sole remaining call site with a g.cmdExec-gated branch before this fix,
// now routes through g.commandRunner() unconditionally, for real -- not just
// that IsDirtyWithHint's existing tests above still pass. runGitCommand backs
// ~25 production call sites (RenameBranch, stageAndCommit,
// StageAllExceptScaffolding, HasStagedChanges, IsDirtyWithHint, and every
// worktree_ops.go worktree add/remove/prune/list call), so this is the seam
// Phase 2's RemoteWorktreeOps depends on actually being live.
func TestRunGitCommand_UsesInjectedCommandRunner(t *testing.T) {
	spy := &gitSpyCommandRunner{runOut: []byte("output\n")}
	g := NewGitWorktreeFromStorageWithExecutor(
		"/fake/repo", "/fake/worktree", "test-session", "test-branch", "",
		WithCommandRunner(spy),
	)

	out, err := g.runGitCommand("/fake/worktree", "status", "--porcelain")
	if err != nil {
		t.Fatalf("runGitCommand returned error: %v", err)
	}
	if out != "output\n" {
		t.Errorf("runGitCommand output = %q, want %q", out, "output\n")
	}

	if len(spy.runCalls) != 1 {
		t.Fatalf("spy.Run call count = %d, want 1 (runGitCommand should route through the injected CommandRunner unconditionally)", len(spy.runCalls))
	}
	call := spy.runCalls[0]
	wantArgs := []string{"status", "--porcelain"}
	if call.dir != "/fake/worktree" || call.name != "git" || len(call.args) != len(wantArgs) {
		t.Fatalf("spy.Run call = %+v, want {dir: \"/fake/worktree\", name: \"git\", args: %v}", call, wantArgs)
	}
	for i, arg := range wantArgs {
		if call.args[i] != arg {
			t.Errorf("spy.Run args[%d] = %q, want %q", i, call.args[i], arg)
		}
	}
}
