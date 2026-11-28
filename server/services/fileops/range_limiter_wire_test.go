package fileops

import (
	"context"
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
	"github.com/tstapler/stapler-squad/server/services/fsops"
)

type rangeWireHandler struct {
	sessionv1connect.UnimplementedSessionServiceHandler
	files *FileService
}

func (h rangeWireHandler) ReadFileRange(ctx context.Context, req *connect.Request[sessionv1.ReadFileRangeRequest]) (*connect.Response[sessionv1.ReadFileRangeResponse], error) {
	return h.files.ReadFileRange(ctx, req)
}

// The range caps must key on the connection's peer address: two peers must not share one budget.
func TestRangeLimiter_ShouldKeyOnWirePeerAddr_When_FileServiceServesReadFileRange(t *testing.T) {
	t.Parallel()
	mem := fsops.NewMemBackend()
	mem.AddFile(rangeTestDir, "a.txt", []byte("x\n"), readCorpusMTime)
	ops := NewFileOps(FilePolicy{Workspace: anySessionProvider{rangeTestDir}, Flags: &stubFilesFlags{rangeRead: true}}, mem)
	gate := make(chan struct{})
	admitted := make(chan struct{}, 64)
	mem.SetOnRead(func(n int) {
		if n == 512 { // the sniff read, inside the limiter slot
			select {
			case admitted <- struct{}{}:
			default:
			}
			<-gate
		}
	})
	mux := http.NewServeMux()
	mux.Handle(sessionv1connect.NewSessionServiceHandler(rangeWireHandler{files: &FileService{ops: ops, ranges: newRangeLimiter()}}))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.RemoteAddr = r.Header.Get("X-Test-Peer") // stands in for distinct client machines
		mux.ServeHTTP(w, r)
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	var held sync.WaitGroup
	var once sync.Once
	release := func() { once.Do(func() { close(gate) }) }
	// LIFO: open the gate, wait for the held calls, close the server, cancel, stop analysis.
	t.Cleanup(ops.analysis.Close)
	t.Cleanup(cancel)
	t.Cleanup(srv.Close)
	t.Cleanup(held.Wait)
	t.Cleanup(release)

	client := sessionv1connect.NewSessionServiceClient(srv.Client(), srv.URL)
	call := func(ctx context.Context, session, peer string) error {
		req := connect.NewRequest(withSession(linesReq("a.txt", 1, sessionv1.RangeDir_RANGE_DIR_FORWARD, 5, 0), session))
		req.Header().Set("X-Test-Peer", peer)
		_, err := client.ReadFileRange(ctx, req)
		return err
	}
	hold := func(session, peer string) {
		for range maxRangesPerClient {
			held.Add(1)
			go func() {
				defer held.Done()
				assert.NoError(t, call(ctx, session, peer))
			}()
		}
		for range maxRangesPerClient {
			select {
			case <-admitted:
			case <-ctx.Done():
				require.FailNow(t, "held calls were not admitted")
			}
		}
	}
	hold("s1", "198.51.100.1:4000")
	hold("s2", "198.51.100.1:4001")

	short, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()
	err := call(short, "s3", "198.51.100.1:5000")
	require.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))
	assert.Equal(t, string(rangeBusyPerIP), rangeBusyReason(t, err))

	// Another peer has its own budget: it is admitted rather than refused.
	held.Add(1)
	go func() {
		defer held.Done()
		assert.NoError(t, call(ctx, "s1", "198.51.100.2:5000"))
	}()
	select {
	case <-admitted:
	case <-time.After(5 * time.Second):
		assert.Fail(t, "a second peer was not admitted")
	}
	release()
	held.Wait()
}
