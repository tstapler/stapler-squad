package services

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
)

// batchStub serves ListFiles with behavior chosen by SessionId: "missing" fails
// NotFound, "huge" returns a body over the per-item cap, "gate" blocks until the
// test releases it (or the call's context ends), anything else echoes.
type batchStub struct {
	sessionv1connect.UnimplementedSessionServiceHandler
	hits     atomic.Int32
	cur, max atomic.Int32
	started  chan struct{}
	release  chan struct{}
	canceled chan string
}

func newBatchStub() *batchStub {
	return &batchStub{started: make(chan struct{}, 64), release: make(chan struct{}), canceled: make(chan string, 64)}
}

func (s *batchStub) ListFiles(ctx context.Context, req *connect.Request[sessionv1.ListFilesRequest]) (*connect.Response[sessionv1.ListFilesResponse], error) {
	s.hits.Add(1)
	id := req.Msg.SessionId
	switch id {
	case "missing":
		return nil, connect.NewError(connect.CodeNotFound, assert.AnError)
	case "huge":
		return connect.NewResponse(&sessionv1.ListFilesResponse{BasePath: strings.Repeat("x", batchMaxItemResponseBytes+1)}), nil
	case "gate", "gate-peer":
		n := s.cur.Add(1)
		defer s.cur.Add(-1)
		for {
			m := s.max.Load()
			if n <= m || s.max.CompareAndSwap(m, n) {
				break
			}
		}
		s.started <- struct{}{}
		select {
		case <-s.release:
		case <-ctx.Done():
			s.canceled <- id
			return nil, ctx.Err()
		}
	}
	return connect.NewResponse(&sessionv1.ListFilesResponse{BasePath: "ok:" + id}), nil
}

type batchHarness struct {
	stub       *batchStub
	dispatched atomic.Int32
	srv        *httptest.Server
}

// newBatchHarness serves the batch handler; the dispatcher enforces a cs_auth
// cookie on every call it sees, standing in for the real auth middleware.
func newBatchHarness(t *testing.T) *batchHarness {
	t.Helper()
	h := &batchHarness{stub: newBatchStub()}
	_, connectHandler := sessionv1connect.NewSessionServiceHandler(h.stub)
	dispatcher := http.StripPrefix("/api", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.dispatched.Add(1)
		if r.Header.Get("Cookie") != "cs_auth=tok" {
			http.Error(w, "login required", http.StatusUnauthorized)
			return
		}
		connectHandler.ServeHTTP(w, r)
	}))
	handler := NewUnaryBatchHandler(func(string) http.Handler { return dispatcher })
	h.srv = httptest.NewServer(WithRequestRecord(ListenerLocal, false)(handler))
	t.Cleanup(h.srv.Close)
	return h
}

func listFilesCall(t *testing.T, id uint32, sessionID string) batchCall {
	t.Helper()
	return batchCall{ID: id, Method: listFilesMethod, Body: mustProto(t, &sessionv1.ListFilesRequest{SessionId: sessionID})}
}

func (h *batchHarness) post(ctx context.Context, t *testing.T, cookie string, calls ...batchCall) (*http.Response, []batchResult) {
	t.Helper()
	body, err := json.Marshal(batchRequest{Calls: calls})
	require.NoError(t, err)
	return h.postRaw(ctx, t, cookie, body)
}

func (h *batchHarness) postRaw(ctx context.Context, t *testing.T, cookie string, body []byte) (*http.Response, []batchResult) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.srv.URL+UnaryBatchPath, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	var out batchResponse
	if resp.StatusCode == http.StatusOK {
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	}
	return resp, out.Results
}

func resultByID(t *testing.T, results []batchResult, id uint32) batchResult {
	t.Helper()
	for _, r := range results {
		if r.ID == id {
			return r
		}
	}
	require.Failf(t, "missing result", "id %d", id)
	return batchResult{}
}

func decodeBatchListFiles(t *testing.T, r batchResult) string {
	t.Helper()
	require.Nil(t, r.Error)
	var out sessionv1.ListFilesResponse
	require.NoError(t, proto.Unmarshal(r.Body, &out))
	return out.BasePath
}

func TestUnaryBatch_should_DispatchEachCallThroughTheChain_When_AllAllowed(t *testing.T) {
	h := newBatchHarness(t)
	resp, results := h.post(context.Background(), t, "cs_auth=tok", listFilesCall(t, 1, "a"), listFilesCall(t, 2, "b"), listFilesCall(t, 3, "c"))

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ok:a", decodeBatchListFiles(t, resultByID(t, results, 1)))
	assert.Equal(t, "ok:b", decodeBatchListFiles(t, resultByID(t, results, 2)))
	assert.Equal(t, "ok:c", decodeBatchListFiles(t, resultByID(t, results, 3)))
	assert.EqualValues(t, 3, h.dispatched.Load(), "every item re-enters the dispatcher")
}

func TestUnaryBatch_should_RecheckAuthPerItem_When_CookieIsMissing(t *testing.T) {
	h := newBatchHarness(t)
	resp, results := h.post(context.Background(), t, "", listFilesCall(t, 1, "a"), listFilesCall(t, 2, "b"))

	require.Equal(t, http.StatusOK, resp.StatusCode, "the envelope succeeds; each item carries its own verdict")
	for _, id := range []uint32{1, 2} {
		r := resultByID(t, results, id)
		require.NotNil(t, r.Error)
		assert.Equal(t, "unauthenticated", r.Error.Code)
	}
	assert.EqualValues(t, 2, h.dispatched.Load())
	assert.EqualValues(t, 0, h.stub.hits.Load(), "no handler ran without auth")
}

func TestUnaryBatch_should_DenyMethodsOutsideTheAllowlist_And_NeverDispatchThem(t *testing.T) {
	h := newBatchHarness(t)
	denied := []string{fileContentMethod, writeFileMethod, "/session.v1.SessionService/GetSessionDiff", "/session.v1.SessionService/GetVCSStatus", "/no.such/Method"}
	calls := make([]batchCall, 0, len(denied)+1)
	calls = append(calls, listFilesCall(t, 100, "ok"))
	for i, m := range denied {
		calls = append(calls, batchCall{ID: uint32(i + 1), Method: m})
	}
	_, results := h.post(context.Background(), t, "cs_auth=tok", calls...)

	for i := range denied {
		r := resultByID(t, results, uint32(i+1))
		assert.True(t, r.Fallback, denied[i])
		require.NotNil(t, r.Error, denied[i])
		assert.Equal(t, codePermissionDenied, r.Error.Code)
	}
	assert.Equal(t, "ok:ok", decodeBatchListFiles(t, resultByID(t, results, 100)))
	assert.EqualValues(t, 1, h.dispatched.Load(), "only the allowed item reached the dispatcher")
}

func TestUnaryBatch_should_KeepBatchAllowedASubsetOfWsAllowedWithoutSlowOrGuardedMethods(t *testing.T) {
	for m := range batchAllowed {
		assert.True(t, wsUnaryMethodAllowed(m), "%s must also be in wsUnaryAllowed", m)
	}
	for _, slow := range []string{
		sessionv1connect.SessionServiceGetSessionDiffProcedure,
		sessionv1connect.SessionServiceGetFileContentProcedure,
		sessionv1connect.SessionServiceGetVCSStatusProcedure,
	} {
		assert.False(t, batchMethodAllowed(slow), "%s is slow or bulk and must travel alone", slow)
	}
}

func TestUnaryBatch_should_MatchTheSharedClientFixture(t *testing.T) {
	var a unaryAllowlist
	loadFixture(t, "ws-unary-allowlist.json", &a)
	bare := make([]string, 0, len(batchAllowed))
	for m := range batchAllowed {
		bare = append(bare, strings.TrimPrefix(m, "/session.v1.SessionService/"))
	}
	assert.ElementsMatch(t, a.BatchAllowed, bare)
}

func TestUnaryBatch_should_ReturnPerItemErrors_When_OneCallFails(t *testing.T) {
	h := newBatchHarness(t)
	resp, results := h.post(context.Background(), t, "cs_auth=tok", listFilesCall(t, 1, "a"), listFilesCall(t, 2, "missing"), listFilesCall(t, 3, "c"))

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ok:a", decodeBatchListFiles(t, resultByID(t, results, 1)))
	failed := resultByID(t, results, 2)
	require.NotNil(t, failed.Error)
	assert.Equal(t, "not_found", failed.Error.Code)
	assert.False(t, failed.Fallback)
	assert.Equal(t, "ok:c", decodeBatchListFiles(t, resultByID(t, results, 3)))
}

func TestUnaryBatch_should_RejectBadEnvelopes(t *testing.T) {
	h := newBatchHarness(t)
	ctx := context.Background()

	tooMany := make([]batchCall, batchMaxCalls+1)
	for i := range tooMany {
		tooMany[i] = listFilesCall(t, uint32(i+1), "a")
	}
	resp, _ := h.post(ctx, t, "cs_auth=tok", tooMany...)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "17 calls")

	resp, _ = h.post(ctx, t, "cs_auth=tok")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "no calls")

	resp, _ = h.post(ctx, t, "cs_auth=tok", listFilesCall(t, 1, "a"), listFilesCall(t, 1, "b"))
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "duplicate ids")

	resp, _ = h.postRaw(ctx, t, "cs_auth=tok", []byte("{not json"))
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed")

	huge := listFilesCall(t, 1, strings.Repeat("x", batchMaxRequestBytes))
	resp, _ = h.post(ctx, t, "cs_auth=tok", huge)
	assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode, "over 256 KiB")
	assert.EqualValues(t, 0, h.dispatched.Load(), "an invalid envelope dispatches nothing")
}

func TestUnaryBatch_should_RejectNonPost_And_UnstampedRequests(t *testing.T) {
	h := newBatchHarness(t)
	resp, err := http.Get(h.srv.URL + UnaryBatchPath)
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)

	rec := httptest.NewRecorder()
	NewUnaryBatchHandler(func(string) http.Handler { return nil }).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, UnaryBatchPath, strings.NewReader("{}")))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code, "no listener record fails closed")
}

func TestUnaryBatch_should_FallBackPerItem_When_ItemResponseExceedsCap(t *testing.T) {
	h := newBatchHarness(t)
	_, results := h.post(context.Background(), t, "cs_auth=tok", listFilesCall(t, 1, "a"), listFilesCall(t, 2, "huge"))

	assert.Equal(t, "ok:a", decodeBatchListFiles(t, resultByID(t, results, 1)))
	big := resultByID(t, results, 2)
	assert.True(t, big.Fallback)
	assert.Empty(t, big.Body)
}

func TestCapBatchResponse_should_FallBackItemsPastTheTotalCap(t *testing.T) {
	chunk := make([]byte, batchMaxTotalResponseBytes/2+1)
	results := []batchResult{{ID: 1, Body: chunk}, {ID: 2, Body: chunk}, {ID: 3}}
	capBatchResponse(results)

	assert.False(t, results[0].Fallback)
	assert.True(t, results[1].Fallback)
	assert.Empty(t, results[1].Body)
}

func TestUnaryBatch_should_IsolateItemDeadlines_When_OneCallTimesOut(t *testing.T) {
	h := newBatchHarness(t)
	gated := listFilesCall(t, 1, "gate")
	gated.TimeoutMs = 20
	_, results := h.post(context.Background(), t, "cs_auth=tok", gated, listFilesCall(t, 2, "b"))

	require.NotNil(t, resultByID(t, results, 1).Error, "the gated call hit its own deadline")
	assert.Equal(t, "ok:b", decodeBatchListFiles(t, resultByID(t, results, 2)), "its peer is unaffected")
}

func TestUnaryBatch_should_CancelHandlers_When_ClientDropsTheRequest(t *testing.T) {
	h := newBatchHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	body, err := json.Marshal(batchRequest{Calls: []batchCall{listFilesCall(t, 1, "gate")}})
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.srv.URL+UnaryBatchPath, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Cookie", "cs_auth=tok")
	done := make(chan struct{})
	go func() {
		defer close(done)
		if resp, doErr := http.DefaultClient.Do(req); doErr == nil {
			_ = resp.Body.Close()
		}
	}()

	<-h.stub.started
	cancel()
	assert.Equal(t, "gate", <-h.stub.canceled)
	<-done
}

func TestUnaryBatch_should_BoundServerConcurrency(t *testing.T) {
	h := newBatchHarness(t)
	calls := make([]batchCall, batchMaxCalls)
	for i := range calls {
		calls[i] = listFilesCall(t, uint32(i+1), "gate")
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		h.post(context.Background(), t, "cs_auth=tok", calls...)
	}()

	for i := 0; i < batchMaxConcurrency; i++ {
		<-h.stub.started
	}
	assert.EqualValues(t, batchMaxConcurrency, h.stub.cur.Load(), "the pool is full and the rest are queued")
	close(h.stub.release)
	wg.Wait()
	assert.LessOrEqual(t, h.stub.max.Load(), int32(batchMaxConcurrency))
	assert.EqualValues(t, batchMaxCalls, h.stub.hits.Load())
}
