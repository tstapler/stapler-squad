package backlog

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	logpkg "github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// Handing the selector (anything but *ConfiguredWorkflowEngine) to the
// lifecycle listener silently disables gate-context resolution, so it must say so.
func TestBacklogLifecycleListener_NonConfiguredEngine_LogsGateContextDegrade(t *testing.T) {
	var buf bytes.Buffer
	prev := logpkg.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { logpkg.SetSlogDefaultForTest(prev) })

	selector := session.NewSwitchableWorkflowEngine(session.NewDefaultWorkflowEngine(), nil, nil)
	l := &BacklogLifecycleListener{}
	l.SetWorkflowEngine(selector)

	_, ok := l.resolveReviewGateContext(session.BacklogStatusReady, session.BacklogStatusQueued)
	require.False(t, ok)
	_, _, ok = l.resolveCustomCheckGateContext(session.BacklogStatusReady, session.BacklogStatusQueued)
	require.False(t, ok)

	require.Contains(t, buf.String(), "not a *ConfiguredWorkflowEngine")
	require.Contains(t, buf.String(), "SwitchableWorkflowEngine")
	require.Equal(t, 1, bytes.Count(buf.Bytes(), []byte("not a *ConfiguredWorkflowEngine")), "logged once")
}

func TestBacklogLifecycleListener_NilEngine_DoesNotLogDegrade(t *testing.T) {
	var buf bytes.Buffer
	prev := logpkg.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { logpkg.SetSlogDefaultForTest(prev) })

	l := &BacklogLifecycleListener{}
	_, ok := l.resolveReviewGateContext(session.BacklogStatusReady, session.BacklogStatusQueued)
	require.False(t, ok)
	require.NotContains(t, buf.String(), "not a *ConfiguredWorkflowEngine")
}
