package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/fsops"
	"github.com/tstapler/stapler-squad/session/domain"
)

// anySessionProvider maps every session id to the same worktree.
type anySessionProvider struct{ dir string }

func (p anySessionProvider) GetWorkspace(string) (domain.Workspace, error) {
	return domain.Workspace{ExistingDir: p.dir}, nil
}

func rangeBusyReason(t *testing.T, err error) string {
	t.Helper()
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	require.Equal(t, connect.CodeResourceExhausted, ce.Code())
	for _, d := range ce.Details() {
		v, derr := d.Value()
		require.NoError(t, derr)
		if busy, ok := v.(*sessionv1.RangeBusyDetail); ok {
			return busy.Reason
		}
	}
	t.Fatal("no RangeBusyDetail on the refusal")
	return ""
}

func TestReadRange_ShouldRefuseWithRangeBusy_When_FifthCallPerClientOrNinthPerIP(t *testing.T) {
	mem := fsops.NewMemBackend()
	mem.AddFile(rangeTestDir, "a.txt", []byte("x\n"), readCorpusMTime)
	ops := NewFileOps(FilePolicy{Workspace: anySessionProvider{rangeTestDir}, Flags: &stubFilesFlags{rangeRead: true}}, mem)
	t.Cleanup(ops.analysis.Close)
	svc := &FileService{ops: ops, ranges: newRangeLimiter()}

	// admitted gets one token per call that holds both limiter slots: the sniff
	// read runs inside the slot, so it is the only point where "held" is exact.
	// (perIP.total alone is not: Acquire bumps perIP before perClient.)
	gate := make(chan struct{})
	var openOnce sync.Once
	openGate := func() { openOnce.Do(func() { close(gate) }) }
	admitted := make(chan struct{}, 64)
	t.Cleanup(openGate) // registered after analysis.Close, so it runs first
	mem.SetOnRead(func(n int) {
		if n == 512 { // the sniff read, inside the limiter slot
			select {
			case admitted <- struct{}{}:
			default:
			}
			<-gate
		}
	})
	call := func(session string) error {
		_, err := svc.ReadFileRange(context.Background(), connect.NewRequest(
			withSession(linesReq("a.txt", 1, sessionv1.RangeDir_RANGE_DIR_FORWARD, 5, 0), session)))
		return err
	}
	var wg sync.WaitGroup
	hold := func(session string, n int) {
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); assert.NoError(t, call(session)) }()
		}
	}
	// refused runs call off the test goroutine: if a cap regressed the call is
	// admitted and parks on the gate, which must fail the test, not hang it.
	refused := func(session, msg string) {
		res := make(chan error, 1)
		go func() { res <- call(session) }()
		select {
		case err := <-res:
			if err == nil {
				require.FailNow(t, "call was admitted when it should be refused", msg)
			}
			assert.Equal(t, msg, rangeBusyReason(t, err))
		case <-time.After(5 * time.Second):
			require.FailNow(t, "call was admitted when it should be refused", msg)
		}
	}
	waitAdmitted := func(n int) {
		for i := 0; i < n; i++ {
			select {
			case <-admitted:
			case <-time.After(10 * time.Second):
				require.FailNow(t, "held calls were not admitted")
			}
		}
	}

	hold("s1", maxRangesPerClient)
	waitAdmitted(maxRangesPerClient)
	refused("s1", string(rangeBusyPerClient)) // fifth call from one client

	hold("s2", maxRangesPerClient)
	waitAdmitted(maxRangesPerClient)
	refused("s3", string(rangeBusyPerIP)) // ninth call from one IP

	openGate()
	wg.Wait()
	assert.NoError(t, call("s1"), "slots are released")
}

func withSession(r *sessionv1.ReadFileRangeRequest, id string) *sessionv1.ReadFileRangeRequest {
	r.SessionId = id
	return r
}
