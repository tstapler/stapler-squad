//go:build harness

package services

// Headless triage test harness — no browser or UI required.
//
// Run all phases (fake pool, fast):
//
//	go test -v -tags=harness -run TestTriageHarness ./server/services/
//
// Run a specific phase (fake pool):
//
//	go test -v -tags=harness -run TestTriageHarness/Gate            ./server/services/
//	go test -v -tags=harness -run TestTriageHarness/TriggerAndPoll  ./server/services/
//	go test -v -tags=harness -run TestTriageHarness/ParserRobust    ./server/services/
//	go test -v -tags=harness -run TestTriageHarness/FullFlow        ./server/services/
//
// Run with a real Claude session (requires claude in PATH, ~30s with fast prompt):
//
//	go test -v -tags=harness -run TestTriageHarness_RealClaude      ./server/services/ -timeout 5m

import (
	"bytes"
	"context"
	"errors"
	stdlog "log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	ssqlog "github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/headless"
)

// setupTriageHarness spins up a real BacklogService + ConnectRPC handler
// behind an httptest.Server. Returns a typed client and the service itself.
func setupTriageHarness(t *testing.T, pool headless.PoolClient) (sessionv1connect.BacklogServiceClient, *BacklogService) {
	t.Helper()
	svc := NewBacklogService(createTestStorage(t), nil, nil, nil)
	svc.SetHeadlessPool(pool)
	t.Cleanup(svc.Shutdown)

	mux := http.NewServeMux()
	blPath, blHandler := sessionv1connect.NewBacklogServiceHandler(svc)
	mux.Handle(blPath, blHandler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := sessionv1connect.NewBacklogServiceClient(srv.Client(), srv.URL)
	return client, svc
}

// preambleTriageJSON wraps validTriageJSON() in natural-language preamble — the
// most common real-world LLM output pattern that broke the old parser.
func preambleTriageJSON() string {
	return "Triage complete. Here is my analysis:\n\n```json\n" + validTriageJSON() + "\n```"
}

// pollUntilReady polls GetBacklogItem until status == "ready" or timeout.
func pollUntilReady(t *testing.T, client sessionv1connect.BacklogServiceClient, itemID string) {
	t.Helper()
	require.Eventually(t, func() bool {
		resp, err := client.GetBacklogItem(context.Background(),
			connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: itemID}))
		return err == nil && resp.Msg.Item.Status == "ready"
	}, 5*time.Second, 50*time.Millisecond, "item %s should reach 'ready' status within 5s", itemID)
}

// TestTriageHarness exercises the backlog triage feature end-to-end via the
// ConnectRPC HTTP layer. Each sub-test covers a distinct portion of the flow.
func TestTriageHarness(t *testing.T) {

	// ──────────────────────────────────────────────────────────────────────────
	// Gate: server must reject TriggerTriage when item has no repoPath.
	// This validates the backend precondition that mirrors the UI disabled state.
	// ──────────────────────────────────────────────────────────────────────────
	t.Run("Gate", func(t *testing.T) {
		client, _ := setupTriageHarness(t, &fakeHeadlessPool{response: validTriageJSON()})

		createResp, err := client.CreateBacklogItem(context.Background(),
			connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
				Title:      "gate-test-item",
				Priority:   3,
				SkipTriage: true,
				// RepoPath intentionally empty
			}))
		require.NoError(t, err)
		itemID := createResp.Msg.Item.Id

		_, trigErr := client.TriggerTriage(context.Background(),
			connect.NewRequest(&sessionv1.TriggerTriageRequest{ItemId: itemID}))

		require.Error(t, trigErr, "TriggerTriage must fail when repoPath is empty")
		var connectErr *connect.Error
		require.ErrorAs(t, trigErr, &connectErr)
		assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code(),
			"expected FailedPrecondition, got: %v", connectErr.Code())
		t.Logf("Gate correctly blocked: %v", connectErr.Message())
	})

	// ──────────────────────────────────────────────────────────────────────────
	// TriggerAndPoll: happy path — trigger triage and poll until item is ready.
	// ──────────────────────────────────────────────────────────────────────────
	t.Run("TriggerAndPoll", func(t *testing.T) {
		pool := &fakeHeadlessPool{response: validTriageJSON()}
		client, _ := setupTriageHarness(t, pool)

		repoPath := t.TempDir()
		createResp, err := client.CreateBacklogItem(context.Background(),
			connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
				Title:      "trigger-poll-item",
				Priority:   2,
				RepoPath:   repoPath,
				SkipTriage: true,
			}))
		require.NoError(t, err)
		itemID := createResp.Msg.Item.Id

		trigResp, trigErr := client.TriggerTriage(context.Background(),
			connect.NewRequest(&sessionv1.TriggerTriageRequest{ItemId: itemID}))
		require.NoError(t, trigErr)
		assert.Equal(t, "triage", trigResp.Msg.ItemSession.SessionRole)
		t.Logf("Triage triggered; item session ID: %s", trigResp.Msg.ItemSession.Id)

		pollUntilReady(t, client, itemID)

		getResp, getErr := client.GetBacklogItem(context.Background(),
			connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: itemID}))
		require.NoError(t, getErr)
		require.NotEmpty(t, getResp.Msg.Item.ItemSessions, "item must have at least one item session")

		is := getResp.Msg.Item.ItemSessions[0]
		assert.NotNil(t, is.EndedAt, "item session should be ended after triage completes")
		assert.Equal(t, "test summary", is.TriageResult.Summary)
		require.Len(t, is.TriageResult.Suggestions, 1)
		assert.Equal(t, "do X", is.TriageResult.Suggestions[0].Text)
		assert.Equal(t, 1, pool.callCount(), "headless pool should have been called exactly once")
		t.Logf("Triage result: summary=%q, tasks=%d", is.TriageResult.Summary, len(is.TriageResult.Tasks))
	})

	// ──────────────────────────────────────────────────────────────────────────
	// ParserRobust: pool returns preamble text before the JSON block — validates
	// the brace-scan fix in ParseHeadlessTriageResult.
	// ──────────────────────────────────────────────────────────────────────────
	t.Run("ParserRobust", func(t *testing.T) {
		pool := &fakeHeadlessPool{response: preambleTriageJSON()}
		client, _ := setupTriageHarness(t, pool)

		repoPath := t.TempDir()
		createResp, err := client.CreateBacklogItem(context.Background(),
			connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
				Title:      "preamble-item",
				Priority:   3,
				RepoPath:   repoPath,
				SkipTriage: true,
			}))
		require.NoError(t, err)
		itemID := createResp.Msg.Item.Id

		_, trigErr := client.TriggerTriage(context.Background(),
			connect.NewRequest(&sessionv1.TriggerTriageRequest{ItemId: itemID}))
		require.NoError(t, trigErr)

		pollUntilReady(t, client, itemID)

		getResp, _ := client.GetBacklogItem(context.Background(),
			connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: itemID}))
		is := getResp.Msg.Item.ItemSessions[0]
		assert.Equal(t, "test summary", is.TriageResult.Summary,
			"parser must extract JSON even when LLM output has a preamble")
		assert.Equal(t, "ready", getResp.Msg.Item.Status,
			"item must reach 'ready' when preamble-wrapped JSON is parsed correctly")
		t.Logf("Parser robustness confirmed: summary=%q", is.TriageResult.Summary)
	})

	// ──────────────────────────────────────────────────────────────────────────
	// FullFlow: create without repoPath → gate blocks → set repoPath → triage
	// succeeds. Mirrors the exact user journey that was broken before the fix.
	// ──────────────────────────────────────────────────────────────────────────
	t.Run("FullFlow", func(t *testing.T) {
		pool := &fakeHeadlessPool{response: validTriageJSON()}
		client, _ := setupTriageHarness(t, pool)

		// Step 1: Create item without repoPath (via empty-state form path)
		createResp, err := client.CreateBacklogItem(context.Background(),
			connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
				Title:      "full-flow-item",
				Priority:   1,
				SkipTriage: true,
			}))
		require.NoError(t, err)
		itemID := createResp.Msg.Item.Id
		t.Logf("[1/5] Created item %s (no repoPath)", itemID)

		// Step 2: Verify gate blocks triage
		_, gateErr := client.TriggerTriage(context.Background(),
			connect.NewRequest(&sessionv1.TriggerTriageRequest{ItemId: itemID}))
		require.Error(t, gateErr, "[2/5] Gate must block triage when repoPath is empty")
		t.Log("[2/5] Gate correctly blocked triage (no repoPath)")

		// Step 3: Update item with repoPath (user fills in the field)
		repoPath := t.TempDir()
		_, updateErr := client.UpdateBacklogItem(context.Background(),
			connect.NewRequest(&sessionv1.UpdateBacklogItemRequest{
				ItemId:   itemID,
				RepoPath: repoPath,
			}))
		require.NoError(t, updateErr)
		t.Logf("[3/5] Set repoPath to %s", repoPath)

		// Step 4: Trigger triage — should now succeed
		trigResp, trigErr := client.TriggerTriage(context.Background(),
			connect.NewRequest(&sessionv1.TriggerTriageRequest{ItemId: itemID}))
		require.NoError(t, trigErr, "[4/5] TriggerTriage must succeed after repoPath is set")
		t.Logf("[4/5] Triage triggered; session role=%s", trigResp.Msg.ItemSession.SessionRole)

		// Step 5: Poll and verify completion
		pollUntilReady(t, client, itemID)
		getResp, _ := client.GetBacklogItem(context.Background(),
			connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: itemID}))
		assert.Equal(t, "ready", getResp.Msg.Item.Status)
		assert.Equal(t, "test summary", getResp.Msg.Item.ItemSessions[0].TriageResult.Summary)
		t.Log("[5/5] Full triage flow completed — item is ready with triage result")
	})
}

// checkPoolStartAllowed skips the test if the process environment blocks starting
// a subprocess with Setsid=true (what the headless pool requires for isolation).
// Previously this failed with EPERM when Setsid+Setpgid were set simultaneously
// (a Linux kernel restriction); the executor was fixed to skip Setpgid when Setsid
// is active, so this pre-check now only fires in truly sandboxed environments.
func checkPoolStartAllowed(t *testing.T) {
	t.Helper()
	truePath, lookErr := exec.LookPath("true")
	if lookErr != nil {
		t.Skip("cannot find /usr/bin/true for pool pre-check")
	}
	// Use the same SysProcAttr the headless pool uses: Setsid only (Setpgid is
	// intentionally omitted because setsid implies a new process group on Linux).
	cmd := exec.Command(truePath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if runErr := cmd.Run(); runErr != nil {
		if strings.Contains(runErr.Error(), "operation not permitted") ||
			strings.Contains(runErr.Error(), "permission denied") {
			t.Skip("subprocess with Setsid blocked by seccomp sandbox — run this test from a real terminal:\n\tgo test -v -tags=harness -run TestTriageHarness_RealClaude ./server/services/ -timeout 5m")
		}
		t.Fatalf("pool start pre-check failed unexpectedly: %v", runErr)
	}
}

// initGitRepo initialises a minimal git repository in dir so claude has a valid
// WorkDir with version control context. Without this, claude may exit immediately
// on some systems that require a git repo for project-context features.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", dir},
		{"-C", dir, "config", "user.email", "test@example.com"},
		{"-C", dir, "config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git %v failed: %v (%s) — cannot run real Claude triage test", args, err, out)
		}
	}
	// A minimal README so the working tree is non-empty.
	if err := os.WriteFile(dir+"/README.md", []byte("# Test Repo\n"), 0o644); err != nil {
		t.Skipf("write README: %v", err)
	}
}

// fastTriagePool wraps a real headless.Pool but replaces BOTH the production
// system prompt and user prompt with minimal versions that produce direct JSON
// output — no sub-agents, no file writes. This keeps the harness test under
// ~60s while still exercising the real claude binary and the full service pipeline.
type fastTriagePool struct {
	pool *headless.Pool
}

// fastTriageSystemPrompt asks claude for direct JSON output with no tool use.
const fastTriageSystemPrompt = `You are a triage assistant. Output ONLY a single JSON object — no other text, no markdown fences, no tool calls. The object must have exactly these fields:
- "summary": string (2-3 sentences)
- "suggestions": array of objects, each with "text" (string) and "rationale" (string) — use [] if none
- "tasks": array of objects, each with "text" (string), "estimate" (string like "1h"), "category" (string like "backend")`

// fastTriageUserPrompt is a minimal prompt that produces immediate JSON output.
// It includes a pre-filled example so claude copies the exact structure.
const fastTriageUserPrompt = `Triage this task and output a JSON result immediately.

Task: Add a GET /health endpoint that returns HTTP 200 with body {"status":"ok"}.

Output a JSON object in exactly this structure (fill in real values):
{"summary":"REPLACE WITH 2-3 sentence summary","suggestions":[],"tasks":[{"text":"REPLACE WITH task name","estimate":"1h","category":"backend"}]}

Rules: suggestions must be an empty array []. tasks must have text, estimate, and category fields. Output JSON only.`

func (p *fastTriagePool) CallBlockingWithOptions(
	ctx context.Context,
	key headless.FeatureKey,
	_, _ string, // discard both production prompts (system and user)
	opts headless.CallOptions,
) (string, error) {
	// Strip WorkDir — the fast prompt doesn't need git context.
	opts.WorkDir = ""
	return p.pool.CallBlockingWithOptions(ctx, key, fastTriageSystemPrompt, fastTriageUserPrompt, opts)
}

// TestTriageHarness_RealClaude exercises the full triage pipeline against a live Claude
// session — real claude binary, no fake pool, no canned JSON.
//
// The pool is wrapped with fastTriagePool which replaces the production agentic
// prompt (4 subagents + file writes) with a minimal direct-JSON prompt so the test
// completes in under 2 minutes instead of 25+.
//
// Run with:
//
//	go test -v -tags=harness -run TestTriageHarness_RealClaude ./server/services/ -timeout 5m
func TestTriageHarness_RealClaude(t *testing.T) {
	// Skip if the process sandbox blocks Setsid — headless pool will fail immediately.
	checkPoolStartAllowed(t)

	realPool, err := headless.NewPool(headless.PoolConfig{
		MaxCallsPerSession:    1,
		MaxConcurrentSessions: 1,
	})
	if errors.Is(err, headless.ErrClaudeNotFound) {
		t.Skip("claude binary not in PATH — skipping real Claude triage test")
	}
	require.NoError(t, err)

	// Wrap the real pool with the fast system prompt override.
	client, _ := setupTriageHarness(t, &fastTriagePool{pool: realPool})

	// Redirect ssqlog.ErrorLog to a buffer so we can surface service errors in t.Log.
	var errBuf bytes.Buffer
	origErrorLog := ssqlog.ErrorLog
	ssqlog.ErrorLog = stdlog.New(&errBuf, "ERROR: ", 0)
	t.Cleanup(func() { ssqlog.ErrorLog = origErrorLog })

	repoPath := t.TempDir()

	createResp, err := client.CreateBacklogItem(context.Background(),
		connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
			Title:       "Add health check endpoint",
			Description: "Add a GET /health endpoint returning {\"status\":\"ok\"}.",
			Priority:    2,
			RepoPath:    repoPath,
			SkipTriage:  true,
		}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id
	t.Logf("Created item %s — triggering real Claude triage (fast prompt, ~30s expected)…", itemID)

	_, trigErr := client.TriggerTriage(context.Background(),
		connect.NewRequest(&sessionv1.TriggerTriageRequest{ItemId: itemID}))
	require.NoError(t, trigErr)

	// Fast-prompt triage should complete in well under 2 minutes.
	require.Eventually(t, func() bool {
		resp, getErr := client.GetBacklogItem(context.Background(),
			connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: itemID}))
		if getErr != nil {
			t.Logf("Poll: GetBacklogItem error: %v", getErr)
			return false
		}
		item := resp.Msg.Item
		if item.Status == "ready" {
			t.Logf("Poll: status=ready ✓")
			return true
		}
		if len(item.ItemSessions) > 0 && item.ItemSessions[0].EndedAt != nil {
			t.Logf("Poll: session ended but status=%q (triage may have failed — check service logs)", item.Status)
			return true // stop polling so we can report a clear failure
		}
		t.Logf("Poll: status=%q sessions=%d", item.Status, len(item.ItemSessions))
		return false
	}, 2*time.Minute, 3*time.Second, "real Claude triage (fast prompt) should complete within 2 minutes")

	getResp, getErr := client.GetBacklogItem(context.Background(),
		connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: itemID}))
	require.NoError(t, getErr)

	item := getResp.Msg.Item
	require.NotEmpty(t, item.ItemSessions, "item must have a triage session")
	is := item.ItemSessions[0]

	// Report service error log entries (only non-empty when triage failed).
	if captured := strings.TrimSpace(errBuf.String()); captured != "" {
		t.Logf("Service error log:\n%s", captured)
	}

	// Report the triage state before any assertions that might panic on nil fields.
	t.Logf("Final state: status=%q, ended=%v, triageResult=%v", item.Status, is.EndedAt != nil, is.TriageResult != nil)
	if is.TriageResult != nil {
		t.Logf("  summary:     %s", is.TriageResult.Summary)
		t.Logf("  tasks:       %d", len(is.TriageResult.Tasks))
		t.Logf("  suggestions: %d", len(is.TriageResult.Suggestions))
		for i, task := range is.TriageResult.Tasks {
			t.Logf("  task[%d]: %s (%s)", i, task.Text, task.Estimate)
		}
	}

	assert.NotNil(t, is.EndedAt, "item session must be ended")
	if !assert.Equal(t, "ready", item.Status, "item should transition to 'ready' after successful triage") {
		t.Fatal("triage did not complete successfully — check service logs for [TriggerTriage] error lines")
	}
	if !assert.NotNil(t, is.TriageResult, "triage result must be set when status is ready") {
		return
	}
	assert.NotEmpty(t, is.TriageResult.Summary, "real Claude triage must produce a non-empty summary")
}
