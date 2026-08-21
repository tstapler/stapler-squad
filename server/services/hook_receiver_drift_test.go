package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// --- isGitCommitOrPushCommand -------------------------------------------------

func TestIsGitCommitOrPushCommand_should_MatchSimpleCommit(t *testing.T) {
	t.Parallel()
	if !isGitCommitOrPushCommand(`git commit -m "fix: something"`) {
		t.Error("expected match on plain git commit")
	}
}

func TestIsGitCommitOrPushCommand_should_MatchSimplePush(t *testing.T) {
	t.Parallel()
	if !isGitCommitOrPushCommand("git push origin backlog/my-branch") {
		t.Error("expected match on plain git push")
	}
}

func TestIsGitCommitOrPushCommand_should_MatchAmend(t *testing.T) {
	t.Parallel()
	if !isGitCommitOrPushCommand("git commit --amend --no-edit") {
		t.Error("expected match on git commit --amend")
	}
}

func TestIsGitCommitOrPushCommand_should_MatchWithGlobalDashCFlag(t *testing.T) {
	t.Parallel()
	if !isGitCommitOrPushCommand(`git -C /worktree commit -m "x"`) {
		t.Error("expected match with -C <path> global flag")
	}
}

func TestIsGitCommitOrPushCommand_should_MatchWithConfigFlag(t *testing.T) {
	t.Parallel()
	if !isGitCommitOrPushCommand(`git -c user.name=agent commit -m "x"`) {
		t.Error("expected match with -c key=value global flag")
	}
}

func TestIsGitCommitOrPushCommand_should_MatchSecondSegmentOfChain(t *testing.T) {
	t.Parallel()
	if !isGitCommitOrPushCommand(`git add -A && git commit -m "x" && git push`) {
		t.Error("expected match on a chained command containing git commit")
	}
}

func TestIsGitCommitOrPushCommand_should_MatchAfterSemicolon(t *testing.T) {
	t.Parallel()
	if !isGitCommitOrPushCommand(`git add -A; git commit -m "x"`) {
		t.Error("expected match after a semicolon-separated segment")
	}
}

func TestIsGitCommitOrPushCommand_should_NotMatchStatus(t *testing.T) {
	t.Parallel()
	if isGitCommitOrPushCommand("git status") {
		t.Error("did not expect match on git status")
	}
}

func TestIsGitCommitOrPushCommand_should_NotMatchDiff(t *testing.T) {
	t.Parallel()
	if isGitCommitOrPushCommand("git diff --stat") {
		t.Error("did not expect match on git diff")
	}
}

func TestIsGitCommitOrPushCommand_should_NotMatchLogWithCommitAsGrepValue(t *testing.T) {
	t.Parallel()
	// "commit" appears in the command line, but not as git's subcommand — it's an
	// argument to --grep on the "log" subcommand. Must not false-positive.
	if isGitCommitOrPushCommand(`git log --grep=commit`) {
		t.Error("did not expect match when 'commit' appears only as a flag value after a different subcommand")
	}
}

func TestIsGitCommitOrPushCommand_should_NotMatchUnrelatedCommand(t *testing.T) {
	t.Parallel()
	if isGitCommitOrPushCommand(`echo "remember to commit and push later"`) {
		t.Error("did not expect match on a command that merely mentions commit/push in a string, not as git's own subcommand")
	}
}

func TestIsGitCommitOrPushCommand_should_NotMatchEmptyString(t *testing.T) {
	t.Parallel()
	if isGitCommitOrPushCommand("") {
		t.Error("did not expect match on empty command")
	}
}

// --- HandlePostToolUseDriftCheck: non-actionable / filtered paths -------------

func newDriftTestReceiver() *HookReceiver {
	h := NewHookReceiver()
	h.SetDriftCheckMinInterval(time.Millisecond) // effectively disable rate limiting in tests
	h.SetDriftThreshold(20)
	return h
}

func postDriftHook(t *testing.T, h *HookReceiver, payload map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/hooks/post-tool-use-drift-check", bytes.NewReader(body))
	req.Header.Set(sessionIDHeader, "test-session")
	rec := httptest.NewRecorder()
	h.HandlePostToolUseDriftCheck(rec, req)
	return rec
}

func TestHandlePostToolUseDriftCheck_should_StaySilent_When_ToolIsNotBash(t *testing.T) {
	t.Parallel()
	h := newDriftTestReceiver()
	h.SetDriftCheckFn(func(string, string) (int, error) {
		t.Fatal("drift check must not run for a non-Bash tool call")
		return 0, nil
	})
	rec := postDriftHook(t, h, map[string]interface{}{
		"cwd":        "/worktree",
		"tool_name":  "Write",
		"tool_input": map[string]interface{}{"file_path": "/worktree/foo.go"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("expected empty body for non-Bash tool, got %q", rec.Body.String())
	}
}

func TestHandlePostToolUseDriftCheck_should_StaySilent_When_BashCommandIsNotGitCommitOrPush(t *testing.T) {
	t.Parallel()
	h := newDriftTestReceiver()
	h.SetDriftCheckFn(func(string, string) (int, error) {
		t.Fatal("drift check must not run for an unrelated Bash command")
		return 0, nil
	})
	rec := postDriftHook(t, h, map[string]interface{}{
		"cwd":        "/worktree",
		"tool_name":  "Bash",
		"tool_input": map[string]interface{}{"command": "go test ./..."},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("expected empty body, got %q", rec.Body.String())
	}
}

func TestHandlePostToolUseDriftCheck_should_StaySilent_When_UnderThreshold(t *testing.T) {
	t.Parallel()
	h := newDriftTestReceiver()
	var called bool
	h.SetDriftCheckFn(func(worktreePath, mainBranch string) (int, error) {
		called = true
		return 5, nil // well under threshold=20
	})
	rec := postDriftHook(t, h, map[string]interface{}{
		"cwd":        "/worktree",
		"tool_name":  "Bash",
		"tool_input": map[string]interface{}{"command": `git commit -m "x"`},
	})
	if !called {
		t.Fatal("expected drift check to run")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("expected no additionalContext under threshold (avoid 'you're fine' noise), got %q", rec.Body.String())
	}
}

func TestHandlePostToolUseDriftCheck_should_FailOpenSilently_When_DriftCheckErrors(t *testing.T) {
	t.Parallel()
	h := newDriftTestReceiver()
	h.SetDriftCheckFn(func(string, string) (int, error) {
		return 0, errors.New("fetch failed: network unreachable")
	})
	rec := postDriftHook(t, h, map[string]interface{}{
		"cwd":        "/worktree",
		"tool_name":  "Bash",
		"tool_input": map[string]interface{}{"command": "git push"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (fail open) even on detection error, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("expected silent fail-open (no additionalContext) on detection error, got %q", rec.Body.String())
	}
}

// --- HandlePostToolUseDriftCheck: actionable path ------------------------------

func TestHandlePostToolUseDriftCheck_should_InjectAdditionalContext_When_OverThreshold(t *testing.T) {
	t.Parallel()
	h := newDriftTestReceiver()
	var gotWorktree, gotBranch string
	h.SetDriftCheckFn(func(worktreePath, mainBranch string) (int, error) {
		gotWorktree = worktreePath
		gotBranch = mainBranch
		return 42, nil // over threshold=20
	})
	rec := postDriftHook(t, h, map[string]interface{}{
		"cwd":        "/worktree",
		"tool_name":  "Bash",
		"tool_input": map[string]interface{}{"command": `git commit -m "feat: thing"`},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotWorktree != "/worktree" {
		t.Errorf("expected drift check to run against cwd /worktree, got %q", gotWorktree)
	}
	if gotBranch != "main" {
		t.Errorf("expected drift check against main, got %q", gotBranch)
	}

	var resp postToolUseHookResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("expected valid JSON response, got %q: %v", rec.Body.String(), err)
	}
	if resp.HookSpecificOutput.HookEventName != "PostToolUse" {
		t.Errorf("expected hookEventName=PostToolUse, got %q", resp.HookSpecificOutput.HookEventName)
	}
	if resp.HookSpecificOutput.AdditionalContext == "" {
		t.Fatal("expected non-empty additionalContext when over threshold")
	}
	if !containsAll(resp.HookSpecificOutput.AdditionalContext, "42", "BUG-044", "git fetch origin") {
		t.Errorf("additionalContext missing expected content: %q", resp.HookSpecificOutput.AdditionalContext)
	}
}

func TestHandlePostToolUseDriftCheck_should_RateLimitRepeatChecks_When_CalledWithinInterval(t *testing.T) {
	t.Parallel()
	h := NewHookReceiver()
	h.SetDriftCheckMinInterval(time.Hour) // long interval: second call within test must be skipped
	h.SetDriftThreshold(1)

	calls := 0
	h.SetDriftCheckFn(func(string, string) (int, error) {
		calls++
		return 999, nil
	})

	payload := map[string]interface{}{
		"cwd":        "/worktree",
		"tool_name":  "Bash",
		"tool_input": map[string]interface{}{"command": "git push"},
	}
	first := postDriftHook(t, h, payload)
	second := postDriftHook(t, h, payload)

	if calls != 1 {
		t.Errorf("expected exactly 1 drift check call across two rapid commits to the same worktree, got %d", calls)
	}
	if first.Body.Len() == 0 {
		t.Error("expected the first (unrate-limited) call to produce additionalContext")
	}
	if second.Body.Len() != 0 {
		t.Error("expected the second (rate-limited) call to stay silent rather than re-fetch")
	}
}

func TestHandlePostToolUseDriftCheck_should_CheckIndependently_When_DifferentWorktrees(t *testing.T) {
	t.Parallel()
	h := NewHookReceiver()
	h.SetDriftCheckMinInterval(time.Hour)
	h.SetDriftThreshold(1)

	calls := 0
	h.SetDriftCheckFn(func(string, string) (int, error) {
		calls++
		return 999, nil
	})

	postDriftHook(t, h, map[string]interface{}{
		"cwd": "/worktree-a", "tool_name": "Bash",
		"tool_input": map[string]interface{}{"command": "git push"},
	})
	postDriftHook(t, h, map[string]interface{}{
		"cwd": "/worktree-b", "tool_name": "Bash",
		"tool_input": map[string]interface{}{"command": "git push"},
	})

	if calls != 2 {
		t.Errorf("expected rate limiting to be scoped per-worktree, got %d calls for two distinct worktrees", calls)
	}
}

func containsAll(haystack string, needles ...string) bool {
	for _, n := range needles {
		if !bytes.Contains([]byte(haystack), []byte(n)) {
			return false
		}
	}
	return true
}
