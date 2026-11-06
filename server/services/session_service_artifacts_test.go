package services

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/artifacts"
)

func rescanReq(id string) *connect.Request[sessionv1.RescanSessionArtifactsRequest] {
	return connect.NewRequest(&sessionv1.RescanSessionArtifactsRequest{Id: id})
}

func TestRescanSessionArtifacts_ShouldResetPositionAndScan_WhenAnyState(t *testing.T) {
	t.Parallel()
	for _, status := range []session.Status{session.Active, session.Paused, session.Stopped} {
		t.Run(status.String(), func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "conv.jsonl")
			line := `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"https://github.com/o/r/pull/4"}]}}` + "\n"
			require.NoError(t, os.WriteFile(path, []byte(line), 0o600))

			inst := &session.Instance{Title: "rescan-me", Status: status}
			inst.SetHistoryInfo("conv", path)

			var scans atomic.Int32
			ae := artifacts.NewArtifactExtractor(
				func(string, string) error { return nil },
				func(string) (string, error) { return "", nil },
				func(p string) (string, bool) { return "rescan-me", p == path },
			)
			ae.OnScanComplete = func(string, *artifacts.SessionArtifactsBlob) { scans.Add(1) }
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			ae.Start(ctx, "")
			ae.OnHistoryFileChanged(path)
			require.Eventually(t, func() bool { return scans.Load() == 1 }, 10*time.Second, 10*time.Millisecond)

			svc := &ArtifactService{rescanner: ae, findInstance: func(string) *session.Instance { return inst }}
			resp, err := svc.RescanSessionArtifacts(ctx, rescanReq("rescan-me"))

			require.NoError(t, err)
			assert.True(t, resp.Msg.ScanStarted)
			// The unchanged file is rescanned only because its position was reset.
			require.Eventually(t, func() bool { return scans.Load() == 2 }, 10*time.Second, 10*time.Millisecond)
		})
	}
}

func TestRescanSessionArtifacts_ShouldFail_WhenSessionMissing(t *testing.T) {
	t.Parallel()
	svc := &ArtifactService{
		rescanner:    artifacts.NewArtifactExtractor(nil, nil, nil),
		findInstance: func(string) *session.Instance { return nil },
	}
	_, err := svc.RescanSessionArtifacts(context.Background(), rescanReq("nope"))
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))

	_, err = svc.RescanSessionArtifacts(context.Background(), rescanReq(""))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestRescanSessionArtifacts_ShouldReportNotStarted_WhenNoTranscriptLinked(t *testing.T) {
	t.Parallel()
	svc := &ArtifactService{
		rescanner:    artifacts.NewArtifactExtractor(nil, nil, nil),
		findInstance: func(string) *session.Instance { return &session.Instance{Title: "unlinked"} },
	}
	resp, err := svc.RescanSessionArtifacts(context.Background(), rescanReq("unlinked"))
	require.NoError(t, err)
	assert.False(t, resp.Msg.ScanStarted)
}

func TestRescanSessionArtifacts_ShouldFail_WhenExtractorNotRunning(t *testing.T) {
	t.Parallel()
	svc := &ArtifactService{findInstance: func(string) *session.Instance { return &session.Instance{} }}
	_, err := svc.RescanSessionArtifacts(context.Background(), rescanReq("x"))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}
