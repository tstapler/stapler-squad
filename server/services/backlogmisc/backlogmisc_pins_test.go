package backlogmisc

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/instance"
)

// deadlineRunner records the context deadline the pool hands the subprocess.
type deadlineRunner struct{ deadline chan time.Time }

func (r deadlineRunner) Run(ctx context.Context, _ []string, _ io.Reader) (io.ReadCloser, func() error, error) {
	d, _ := ctx.Deadline()
	r.deadline <- d
	return io.NopCloser(strings.NewReader(firstCallJSONHS("s", "ok"))), func() error { return nil }, nil
}

// timeout_seconds is capped at MaxCallTimeout, defaults when 0, and is honoured below the cap.
func TestHeadlessService_RunHeadlessCall_should_BoundTheCallDeadline(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		seconds  int32
		min, max time.Duration
	}{
		{"above cap", 100000, headless.MaxCallTimeout - time.Minute, headless.MaxCallTimeout},
		{"default", 0, headless.DefaultCallTimeout - time.Minute, headless.DefaultCallTimeout},
		{"explicit", 30, 20 * time.Second, 30 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := deadlineRunner{deadline: make(chan time.Time, 1)}
			_, client := newHeadlessTestServer(t, headless.NewPoolWithRunner(headless.PoolConfig{}, r))
			stream, err := client.RunHeadlessCall(context.Background(), connect.NewRequest(&sessionv1.RunHeadlessCallRequest{
				FeatureKey: "custom", UserPrompt: "p", TimeoutSeconds: tc.seconds,
			}))
			require.NoError(t, err)
			for stream.Receive() {
			}
			require.NoError(t, stream.Err())
			remaining := time.Until(<-r.deadline)
			assert.LessOrEqual(t, remaining, tc.max)
			assert.Greater(t, remaining, tc.min)
		})
	}
}

func lastStatusEvent(t *testing.T, item *backlogdto.BacklogItemData) backlogdto.BacklogStatusEventData {
	t.Helper()
	require.NotEmpty(t, item.StatusEvents)
	last := item.StatusEvents[0]
	for _, ev := range item.StatusEvents {
		if ev.CreatedAt.After(last.CreatedAt) {
			last = ev
		}
	}
	return last
}

// The debug endpoints attribute a forced transition to the system and an archive to the user.
func TestBacklogDebugMutateHandler_should_AttributeTransitionToSystemAndArchiveToUser(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	h := NewBacklogDebugMutateHandler(storage)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	post := func(path, body string) int {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		return rec.Code
	}

	item, err := storage.CreateBacklogItemForDebug(context.Background(), backlogdto.BacklogItemData{Title: "t", Status: "idea", Priority: 3})
	require.NoError(t, err)

	require.Equal(t, http.StatusNoContent, post("/api/debug/backlog/mutate-transition", `{"itemId":"`+item.ID+`","targetStatus":"review"}`))
	got, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	ev := lastStatusEvent(t, got)
	assert.Equal(t, "review", ev.ToStatus)
	assert.Equal(t, instance.TriggeredBySystem, ev.TriggeredBy)

	require.Equal(t, http.StatusNoContent, post("/api/debug/backlog/mutate-archive", `{"itemId":"`+item.ID+`"}`))
	got, err = storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	ev = lastStatusEvent(t, got)
	assert.Equal(t, "archived", ev.ToStatus)
	assert.Equal(t, instance.TriggeredByUser, ev.TriggeredBy)
}

// A write that cannot be persisted is an Internal error, never a silent success.
func TestLLMBackendService_should_ReturnInternal_When_ConfigCannotBeSaved(t *testing.T) {
	dir := svctest.WithIsolatedStateDir(t)
	config.ResetLiveLLMBackendsForTest()
	t.Cleanup(config.ResetLiveLLMBackendsForTest)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, config.ConfigFileName, "blocker"), 0o755))

	svc := NewLLMBackendService(nil)
	_, err := svc.UpdateLLMBackendSettings(context.Background(), connect.NewRequest(&sessionv1.UpdateLLMBackendSettingsRequest{
		Settings: &sessionv1.LLMBackendSettings{DefaultBackend: "claude"},
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
}
