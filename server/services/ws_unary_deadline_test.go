package services

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func TestUnaryWsBridge_should_ClampClientDeadline_And_CancelHandlerCtx(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal, WithUnaryMaxCallTimeout(40*time.Millisecond))
	c := h.connect(nil)
	// One call asks for an hour, one sends no deadline: the server clamp bounds both.
	sendRequest(t, c, 1, listFilesMethod, unaryRequestMeta{TimeoutMs: 3_600_000}, mustProto(t, &sessionv1.ListFilesRequest{SessionId: "gate:clamp1"}))
	sendRequest(t, c, 3, listFilesMethod, unaryRequestMeta{}, mustProto(t, &sessionv1.ListFilesRequest{SessionId: "gate:clamp2"}))
	for i := 0; i < 2; i++ {
		f := readFrame(t, c)
		require.Equal(t, unaryFrameError, f.Type)
		assert.Equal(t, "deadline_exceeded", f.Error.Code)
	}
	first, second := <-h.stub.canceled, <-h.stub.canceled
	assert.ElementsMatch(t, []string{"gate:clamp1", "gate:clamp2"}, []string{first, second},
		"handler contexts were canceled by the bridge deadline")
}

func TestUnaryWsBridge_should_RunMoreThanSixCallsConcurrently(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal)
	c := h.connect(nil)
	const n = 40
	for i := 0; i < n; i++ {
		sendListFiles(t, c, uint32(2*i+1), "gate:many"+strconv.Itoa(i))
		<-h.stub.started
	}
	assert.EqualValues(t, n, h.bridge.Metrics().Inflight())
	for i := 0; i < n; i++ {
		h.stub.release("gate:many" + strconv.Itoa(i))
	}
	for i := 0; i < n; i++ {
		require.Equal(t, unaryFrameResponse, readFrame(t, c).Type)
	}
}
