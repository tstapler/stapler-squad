package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services"
)

type batchItem struct {
	ID       uint32 `json:"id"`
	Body     string `json:"body"`
	Fallback bool   `json:"fallback"`
	Error    *struct {
		Code string `json:"code"`
	} `json:"error"`
}

// postBatch sends one batch through the real chain; withAuth controls the cookie.
func (h *chainHarness) postBatch(t *testing.T, withAuth bool, calls ...map[string]any) (int, []batchItem) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"calls": calls})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, h.ts.URL+services.UnaryBatchPath, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", h.scheme()+"://"+h.host())
	if withAuth && h.auth {
		req.Header.Set("Cookie", "cs_auth="+chainToken)
	}
	resp, err := h.ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	var out struct {
		Results []batchItem `json:"results"`
	}
	if resp.StatusCode == http.StatusOK {
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	}
	return resp.StatusCode, out.Results
}

func batchCallFor(t *testing.T, id uint32, method string, msg proto.Message) map[string]any {
	t.Helper()
	b, err := proto.Marshal(msg)
	require.NoError(t, err)
	return map[string]any{"id": id, "method": method, "body": base64.StdEncoding.EncodeToString(b), "timeoutMs": 5000}
}

func TestBatchChain_should_ServeAllowedAndDeclineGuardedItems_OnEveryListener(t *testing.T) {
	for _, cc := range chainCases {
		t.Run(cc.name, func(t *testing.T) {
			h := newChainHarness(t, cc)
			status, results := h.postBatch(t, true,
				batchCallFor(t, 1, chainListFiles, &sessionv1.ListFilesRequest{SessionId: "same"}),
				batchCallFor(t, 2, chainWriteFile, &sessionv1.WriteFileRequest{}),
			)

			require.Equal(t, http.StatusOK, status)
			byID := map[uint32]batchItem{}
			for _, r := range results {
				byID[r.ID] = r
			}
			raw, err := base64.StdEncoding.DecodeString(byID[1].Body)
			require.NoError(t, err)
			var got sessionv1.ListFilesResponse
			require.NoError(t, proto.Unmarshal(raw, &got))
			assert.Equal(t, "base:same", got.BasePath)

			assert.True(t, byID[2].Fallback, "a method outside batchAllowed is declined")
			require.NotNil(t, byID[2].Error)
			assert.Equal(t, "permission_denied", byID[2].Error.Code)
			assert.Zero(t, h.stub.writeHits.Load(), "the declined item never reached its handler")
			assert.EqualValues(t, 1, h.stub.listHits.Load())
		})
	}
}

func TestBatchChain_should_RejectUnauthenticatedBatches_When_AuthIsRequired(t *testing.T) {
	for _, cc := range chainCases {
		if !cc.auth {
			continue
		}
		t.Run(cc.name, func(t *testing.T) {
			h := newChainHarness(t, cc)
			status, _ := h.postBatch(t, false, batchCallFor(t, 1, chainListFiles, &sessionv1.ListFilesRequest{SessionId: "x"}))

			assert.NotEqual(t, http.StatusOK, status)
			assert.Zero(t, h.stub.listHits.Load(), "no handler ran without a session")
		})
	}
}

func TestBatchChain_should_RejectTheBatch_When_SessionHasExpired(t *testing.T) {
	h := newChainHarness(t, chainCase{"local-auth", services.ListenerLocal, true})
	h.validator.expired.Store(true)
	status, _ := h.postBatch(t, true, batchCallFor(t, 1, chainListFiles, &sessionv1.ListFilesRequest{SessionId: "x"}))

	assert.NotEqual(t, http.StatusOK, status, "an expired session is rejected by the chain before any item runs")
	assert.Zero(t, h.stub.listHits.Load())
}
