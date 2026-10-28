package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/log"
)

// Request batching over plain HTTP (feature flag batch_unary). One POST carries up
// to batchMaxCalls independent idempotent reads; each is re-dispatched through the
// listener's full handler chain (HostGuard, auth, guards, interceptors), so a batch
// never widens what a direct call may do. Any item the batch cannot serve comes back
// with fallback=true and the client repeats that call on its own.

const (
	// UnaryBatchPath sits under /api like every other bridge.
	UnaryBatchPath = "/api/batch/unary"

	batchMaxCalls              = 16
	batchMaxRequestBytes       = 256 << 10
	batchMaxItemResponseBytes  = 2 << 20
	batchMaxTotalResponseBytes = 8 << 20
	batchMaxConcurrency        = 6
	batchDefaultTimeoutMs      = 30_000
	batchMaxTimeoutMs          = 60_000
)

// batchAllowed is the deny-by-default set of procedures a batch may dispatch: a
// subset of wsUnaryAllowed (reads), minus the slow or bulk ones. One response is
// returned only after every item finishes, so a slow git call or a multi-MB body
// would hold the cheap calls batched with it; those travel alone.
var batchAllowed = map[string]struct{}{
	sessionv1connect.SessionServiceListFilesProcedure:                    {},
	sessionv1connect.SessionServiceStatFileProcedure:                     {},
	sessionv1connect.SessionServiceGetFileTransportCapabilitiesProcedure: {},
	sessionv1connect.SessionServiceListShellsProcedure:                   {},
	sessionv1connect.SessionServiceListWorkflowsProcedure:                {},
	sessionv1connect.SessionServiceListProgramsConfigProcedure:           {},
}

func batchMethodAllowed(method string) bool {
	_, ok := batchAllowed[method]
	return ok
}

type batchCall struct {
	ID        uint32 `json:"id"`
	Method    string `json:"method"`
	Body      []byte `json:"body"` // base64 on the wire
	TimeoutMs int64  `json:"timeoutMs"`
}

type batchRequest struct {
	Calls []batchCall `json:"calls"`
}

// batchResult is one item's outcome: Body on success, Error for the call's own Connect
// failure, Fallback when the batch declined to serve it.
type batchResult struct {
	ID       uint32          `json:"id"`
	Body     []byte          `json:"body,omitempty"`
	Error    *unaryErrorBody `json:"error,omitempty"`
	Fallback bool            `json:"fallback,omitempty"`
}

type batchResponse struct {
	Results []batchResult `json:"results"`
}

// UnaryBatchHandler serves UnaryBatchPath.
type UnaryBatchHandler struct {
	dispatchers UnaryDispatchers
}

// NewUnaryBatchHandler builds the handler over the same per-listener dispatchers the
// unary WebSocket bridge re-enters.
func NewUnaryBatchHandler(dispatchers UnaryDispatchers) *UnaryBatchHandler {
	return &UnaryBatchHandler{dispatchers: dispatchers}
}

func (h *UnaryBatchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	// Requiring JSON forces a CORS preflight on cross-site POSTs (a text/plain form post would not).
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		http.Error(w, "application/json required", http.StatusUnsupportedMediaType)
		return
	}
	rec, ok := RequestRecordFrom(r.Context())
	if !ok || rec.Listener == "" {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	dispatcher := h.dispatchers(rec.Listener)
	if dispatcher == nil {
		http.Error(w, "listener dispatcher not ready", http.StatusServiceUnavailable)
		return
	}
	req, status, err := decodeBatchRequest(w, r)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	results := runBatch(r, dispatcher, req.Calls)
	capBatchResponse(results)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(batchResponse{Results: results}); err != nil {
		log.Debug("[UnaryBatch] write response", "err", err)
	}
}

func decodeBatchRequest(w http.ResponseWriter, r *http.Request) (batchRequest, int, error) {
	var req batchRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, batchMaxRequestBytes)).Decode(&req); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return req, http.StatusRequestEntityTooLarge, errors.New("batch request exceeds 256 KiB")
		}
		return req, http.StatusBadRequest, errors.New("malformed batch request")
	}
	if n := len(req.Calls); n == 0 || n > batchMaxCalls {
		return req, http.StatusBadRequest, errors.New("batch must carry 1 to 16 calls")
	}
	seen := make(map[uint32]struct{}, len(req.Calls))
	for _, c := range req.Calls {
		if _, dup := seen[c.ID]; dup {
			return req, http.StatusBadRequest, errors.New("duplicate call id")
		}
		seen[c.ID] = struct{}{}
	}
	return req, http.StatusOK, nil
}

// runBatch runs the calls with bounded concurrency; results keep request order but
// callers must not rely on completion order.
func runBatch(src *http.Request, dispatcher http.Handler, calls []batchCall) []batchResult {
	results := make([]batchResult, len(calls))
	sem := make(chan struct{}, batchMaxConcurrency)
	var wg sync.WaitGroup
	for i := range calls {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = runBatchCall(src, dispatcher, calls[i])
		}(i)
	}
	wg.Wait()
	return results
}

func batchTimeout(ms int64) time.Duration {
	if ms <= 0 {
		ms = batchDefaultTimeoutMs
	}
	return time.Duration(min(ms, batchMaxTimeoutMs)) * time.Millisecond
}

func runBatchCall(src *http.Request, dispatcher http.Handler, c batchCall) batchResult {
	res := batchResult{ID: c.ID}
	if !batchMethodAllowed(c.Method) {
		res.Fallback = true
		res.Error = &unaryErrorBody{Code: codePermissionDenied, Message: "method not allowed in a batch"}
		return res
	}
	timeout := batchTimeout(c.TimeoutMs)
	ctx, cancel := context.WithTimeout(src.Context(), timeout)
	defer cancel()
	req, err := synthesizeUnaryRequest(ctx, src, c.Method, c.Body, unaryRequestMeta{TimeoutMs: timeout.Milliseconds()})
	if err != nil {
		res.Error = &unaryErrorBody{Code: codeInternal, Message: err.Error()}
		return res
	}
	rec := &unaryRecorder{header: http.Header{}, maxBytes: batchMaxItemResponseBytes}
	if serveGuarded(dispatcher, rec, req, c.Method) {
		res.Error = &unaryErrorBody{Code: codeInternal, Message: "handler panicked"}
		return res
	}
	switch {
	case rec.overflow:
		res.Fallback = true
	case rec.statusOrOK() != http.StatusOK:
		e := rec.errorBody()
		res.Error = &e
	default:
		res.Body = rec.body.Bytes()
	}
	return res
}

func serveGuarded(h http.Handler, w http.ResponseWriter, r *http.Request, method string) (panicked bool) {
	defer func() {
		if p := recover(); p != nil {
			log.Error("[UnaryBatch] handler panic", "method", method, "panic", p)
			panicked = true
		}
	}()
	h.ServeHTTP(w, r)
	return false
}

// capBatchResponse turns the items that would push the response past the total cap
// into fallbacks, so one reply can never grow beyond batchMaxTotalResponseBytes.
func capBatchResponse(results []batchResult) {
	total := 0
	for i := range results {
		wire := base64.StdEncoding.EncodedLen(len(results[i].Body))
		if total+wire > batchMaxTotalResponseBytes {
			results[i] = batchResult{ID: results[i].ID, Fallback: true}
			continue // a dropped item adds nothing, so later small ones can still fit
		}
		total += wire
	}
}
