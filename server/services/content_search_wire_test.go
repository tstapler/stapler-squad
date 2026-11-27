package services

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/session/domain"
)

// The stream caps must key on the connection's peer address as FileService.SearchContent passes it:
// a forwarded header must not split one peer, and two peers must not share one budget.
func TestStreamLimiter_ShouldKeyOnWirePeerAddr_When_FileServiceServesSearchContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	goldenWriteFile(t, dir, "a.txt", "needle\n")
	ws := &goldenWorkspaces{byID: map[string]domain.Workspace{"s1": {RepoRoot: dir, ActiveDir: dir, ExistingDir: dir}}}
	svc := NewFileService(ws)
	svc.SetFlagRegistry(goldenRegistry(map[string]bool{flagsvc.FilesViewerV2FlagName: true}))

	mux := http.NewServeMux()
	mux.Handle(sessionv1connect.NewSessionServiceHandler(goldenFilesHandler{files: svc}))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.RemoteAddr = r.Header.Get("X-Test-Peer") // stands in for distinct client machines
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	release := ws.arm()
	t.Cleanup(release) // runs before srv.Close, which waits for the held handlers

	client := sessionv1connect.NewSessionServiceClient(srv.Client(), srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), goldenCallTimeout)
	t.Cleanup(cancel)
	search := func(ctx context.Context, session, peer string, n int) error {
		req := connect.NewRequest(&sessionv1.SearchContentRequest{
			SessionId: session, ClientId: fmt.Sprint("tab", n),
			Options: &sessionv1.SearchOptions{Pattern: "needle"},
		})
		req.Header().Set("X-Test-Peer", peer)
		req.Header().Set("X-Forwarded-For", fmt.Sprintf("203.0.113.%d", n))
		st, err := client.SearchContent(ctx, req)
		if err != nil {
			return err
		}
		defer func() { _ = st.Close() }()
		for st.Receive() {
		}
		return st.Err()
	}

	var held sync.WaitGroup
	for i := range 3 {
		held.Add(1)
		go func() {
			defer held.Done()
			assert.NoError(t, search(ctx, "s1-hold", fmt.Sprintf("198.51.100.1:%d", 4000+i), i))
		}()
	}
	ws.waitEntered(t, 3)

	short, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()
	err := search(short, "s1", "198.51.100.1:5000", 4)
	require.Error(t, err, "a fourth stream from one peer is refused whatever it forwards")
	require.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))
	assert.Contains(t, goldenErr(err), "SearchBusyDetail 0a067065725f6970", "reason per_ip")
	assert.NoError(t, search(short, "s1", "198.51.100.2:5000", 5), "another peer has its own budget")

	release()
	held.Wait()
}
