package mcp

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/instance"
)

type fakeTerminalScrollback struct {
	mu       sync.Mutex
	seq      uint64
	data     []byte
	lastSeq  uint64
	lastCall string
}

func (f *fakeTerminalScrollback) CurrentSequence(string) uint64 { return f.seq }

func (f *fakeTerminalScrollback) GetRecentBytes(string, int64) ([]byte, error) { return f.data, nil }

func (f *fakeTerminalScrollback) LastEntrySequence(sessionID string, limit int) uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastCall = sessionID
	if limit != 10000 {
		return 0
	}
	return f.lastSeq
}

func TestNewCore_should_ReadTerminalOutputFromInjectedScrollback(t *testing.T) {
	t.Parallel()
	fake := &fakeTerminalScrollback{seq: 3, data: []byte("FROM_FAKE_PORT\n")}
	store := &stubStore{instances: []*instance.Instance{{Title: "s1"}}}
	s := NewCore(store, nil, fake, nil, nil, nil, nil, nil, nil, nil)

	res, err := s.GetTool("read_session_output").Handler(context.Background(), makeToolReq(map[string]interface{}{"session_id": "s1"}))

	require.NoError(t, err)
	assert.Contains(t, toolResultText(t, res), "FROM_FAKE_PORT")
}

func TestCollectCommandOutput_should_ReportLastSequenceFromScrollbackPort(t *testing.T) {
	t.Parallel()
	fake := &fakeTerminalScrollback{seq: 1, data: []byte("done\n"), lastSeq: 42}
	th := &terminalHandlers{scrollback: fake, writeLim: newTokenBucket(10, 10)}

	res := th.collectCommandOutput("s1", &instance.Instance{Title: "s1"}, 5, 50)

	got := parseResult(t, res)
	assert.EqualValues(t, 42, got["last_sequence"])
	fake.mu.Lock()
	defer fake.mu.Unlock()
	assert.Equal(t, "s1", fake.lastCall)
}

func TestNewCore_should_HandInjectedAutoReopenerToReviewVerdicts(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	itemID, sessUUID := setupReviewSession(t, storage)
	reopener := &fakeAutoReopenSpawner{}
	s := NewCore(&stubStore{}, nil, nil, storage, nil, nil, nil, reopener, nil, nil)

	_, err := s.GetTool("submit_review_verdict").Handler(WithSessionUUID(context.Background(), sessUUID), makeToolReq(map[string]interface{}{
		"item_id":  itemID,
		"summary":  "Blocked.",
		"verdicts": verdictsArg("FAIL"),
	}))

	require.NoError(t, err)
	assert.Equal(t, []string{itemID}, reopener.calledWith())
}
