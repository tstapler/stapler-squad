package git

import (
	"os/exec"
	"strings"
	"testing"
	"time"
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

// raceSimulatorExecutor implements executor.Executor for testing the double-checked
// locking invariant in IsDirtyWithHint.  When CombinedOutput is called it runs
// raceSetup first (simulating a concurrent goroutine updating the cache), then
// returns the configured output.
type raceSimulatorExecutor struct {
	output    []byte
	raceSetup func()
}

func (e *raceSimulatorExecutor) Run(_ *exec.Cmd) error              { return nil }
func (e *raceSimulatorExecutor) Output(_ *exec.Cmd) ([]byte, error) { return e.output, nil }
func (e *raceSimulatorExecutor) CombinedOutput(_ *exec.Cmd) ([]byte, error) {
	if e.raceSetup != nil {
		e.raceSetup()
	}
	return e.output, nil
}

// TestIsDirtyWithHint_ReturnsLocallyComputedValue_WhenCacheIsWrittenByRacingGoroutine
// verifies the return-own-observation invariant: IsDirtyWithHint must return the
// locally-computed value, not a re-read of the cache slot.
//
// With atomic.Value the write is unconditional, so a race can't suppress our Store.
// The test simulates a racing goroutine that stores false into the cache WHILE our
// git subprocess is running; our code must still return true (its own observation).
func TestIsDirtyWithHint_ReturnsLocallyComputedValue_WhenCacheIsWrittenByRacingGoroutine(t *testing.T) {
	t.Parallel()
	mock := &raceSimulatorExecutor{
		output: []byte("M file.txt\n"), // our goroutine sees the worktree as dirty
	}

	g := NewGitWorktreeFromStorageWithExecutor(
		"/fake/repo", "/fake/worktree", "test-session", "test-branch", "", mock,
	)

	// The raceSetup closure runs inside CombinedOutput, simulating a concurrent
	// goroutine that stores dirty=false while our git call is "in flight".
	mock.raceSetup = func() {
		g.isDirtyCache.Store(dirtyCacheState{dirty: false, time: time.Now()})
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

// countingErrExecutor always fails CombinedOutput and counts how many times it was
// invoked, simulating `git status` against a worktree directory that no longer exists.
type countingErrExecutor struct {
	calls int
}

func (e *countingErrExecutor) Run(_ *exec.Cmd) error              { return nil }
func (e *countingErrExecutor) Output(_ *exec.Cmd) ([]byte, error) { return nil, nil }
func (e *countingErrExecutor) CombinedOutput(_ *exec.Cmd) ([]byte, error) {
	e.calls++
	return []byte("fatal: cannot change to '/fake/worktree': No such file or directory"), exec.ErrNotFound
}

// TestIsDirtyWithHint_BacksOffAfterError proves that a failing `git status` (e.g. the
// worktree directory is missing — the stale-path-after-rework bug) is cached with a
// backoff TTL rather than re-run on every call: a second call made immediately after a
// failure must return the same error without spawning another subprocess.
func TestIsDirtyWithHint_BacksOffAfterError(t *testing.T) {
	t.Parallel()
	mock := &countingErrExecutor{}
	g := NewGitWorktreeFromStorageWithExecutor(
		"/fake/repo", "/fake/worktree", "test-session", "test-branch", "", mock,
	)
	g.isDirtyCache.Store(dirtyCacheState{}) // zero time = cache invalid

	if _, err := g.IsDirtyWithHint(false); err == nil {
		t.Fatalf("IsDirtyWithHint() error = nil; want an error from the failing git command")
	}
	if mock.calls != 1 {
		t.Fatalf("calls after first (failing) check = %d; want 1", mock.calls)
	}

	if _, err := g.IsDirtyWithHint(false); err == nil {
		t.Fatalf("second IsDirtyWithHint() error = nil; want the cached error")
	}
	if mock.calls != 1 {
		t.Errorf("calls after second check within backoff TTL = %d; want still 1 (no new subprocess spawned)", mock.calls)
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
