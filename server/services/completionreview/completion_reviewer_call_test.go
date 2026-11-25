package completionreview

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

func TestNewHeadlessCompletionReviewCall_should_Fail_When_PoolNotInitialized(t *testing.T) {
	prev := headless.DefaultPool()
	headless.SetDefaultPool(nil)
	t.Cleanup(func() { headless.SetDefaultPool(prev) })

	out, err := NewHeadlessCompletionReviewCall()(context.Background(), "sys", "user")
	require.Error(t, err)
	assert.Equal(t, "headless pool not initialized", err.Error())
	assert.Empty(t, out)
}

func TestNewHeadlessCompletionReviewCall_should_RunTextOnlyInThrowawayDir(t *testing.T) {
	callLog := filepath.Join(t.TempDir(), "call.log")
	script := filepath.Join(t.TempDir(), "fake-claude.sh")
	body := "#!/bin/sh\ncat >/dev/null\nprintf '%s\\n' \"$PWD\" \"$*\" > '" + callLog + "'\n" +
		`printf '%s\n' '{"type":"result","session_id":"s","result":"{\"learnings\":\"ok\"}","total_cost_usd":0}'` + "\n"
	require.NoError(t, os.WriteFile(script, []byte(body), 0o700))

	prev := headless.DefaultPool()
	headless.SetDefaultPool(headless.NewPoolWithRunner(headless.PoolConfig{}, headless.NewShellWrappedProcessRunnerForTesting(script)))
	t.Cleanup(func() { headless.SetDefaultPool(prev) })

	out, err := NewHeadlessCompletionReviewCall()(context.Background(), "sys", "user")
	require.NoError(t, err)
	assert.Equal(t, `{"learnings":"ok"}`, out)

	raw, err := os.ReadFile(callLog)
	require.NoError(t, err)
	lines := strings.SplitN(strings.TrimSpace(string(raw)), "\n", 2)
	require.Len(t, lines, 2)
	assert.Contains(t, lines[0], "ssq-completion-review-")
	_, statErr := os.Stat(lines[0])
	assert.True(t, os.IsNotExist(statErr), "the throwaway work dir is removed after the call")
	assert.Contains(t, lines[1], "--tools  --strict-mcp-config --disable-slash-commands", "the call must be text-only")
}

func TestNewFileMemoryStore_should_ReturnAppendOnlyStore(t *testing.T) {
	t.Parallel()
	_, isFile := NewFileMemoryStore().(fileMemoryStore)
	assert.True(t, isFile)
}

func TestFileMemoryStore_Append_should_CreateGroupReadableButNotWritableDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, fileMemoryStore{dir: dir}.Append(context.Background(), MemoryEntry{SourceItemID: "i", RepoPath: "/repo/x", Content: "x"}))
	info, err := os.Stat(filepath.Join(dir, "completion-reviews"))
	require.NoError(t, err)
	assert.Zero(t, info.Mode().Perm()&0o027, "directory must not be group-writable or world-accessible: %v", info.Mode().Perm())
}

func TestCompletionReviewerLoop_should_DropWaitingEvent_When_CancelledWhileCapFull(t *testing.T) {
	var started atomic.Int32
	release := make(chan struct{})
	r := newTestReviewer(t, func(context.Context, string, string) (string, error) {
		started.Add(1)
		<-release
		return "", nil
	}, &fakeMemoryStore{})
	total := maxConcurrentCompletionReviews + 1
	ch := make(chan *events.Event, total)
	for i := 0; i < total; i++ {
		ch <- doneEvent(richCompletionItem(t, string(rune('a'+i))), events.BacklogChangeStatusTransition, string(domain.BacklogStatusDone))
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	loopDone := make(chan struct{})
	go func() { defer close(loopDone); r.loop(ctx, ch) }()

	wait.RequireEventually(t, func() bool { return started.Load() == maxConcurrentCompletionReviews && len(ch) == 0 }, 2*time.Second, 5*time.Millisecond, "cap should fill with the last event consumed and waiting for a slot")
	cancel()
	close(release)
	wait.RequireEventually(t, func() bool {
		select {
		case <-loopDone:
			return true
		default:
			return false
		}
	}, 2*time.Second, 5*time.Millisecond, "loop should return on cancel")
	assert.EqualValues(t, maxConcurrentCompletionReviews, started.Load(), "the event waiting for a slot is dropped on cancel")
}

func TestMaybeLogStatus_should_LogEveryHundredthAttempt_WithinThePeriod(t *testing.T) {
	t.Parallel()
	for n, wantLogged := range map[int64]bool{50: false, 99: false, 100: true, 101: false, 200: true} {
		r := &completionReviewer{}
		r.attempts.Store(n)
		recent := time.Now().UnixNano()
		r.lastStatusLog.Store(recent)
		r.maybeLogStatus()
		assert.Equal(t, wantLogged, r.lastStatusLog.Load() != recent, "attempts=%d", n)
	}
}
