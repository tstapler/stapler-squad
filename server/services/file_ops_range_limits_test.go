package services

import (
	"context"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// anySessionProvider maps every session id to the same worktree.
type anySessionProvider struct{ dir string }

func (p anySessionProvider) GetWorkspace(string) (session.Workspace, error) {
	return session.Workspace{ExistingDir: p.dir}, nil
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
	mem := newMemBackend()
	mem.AddFile(rangeTestDir, "a.txt", []byte("x\n"), readCorpusMTime)
	ops := NewFileOps(FilePolicy{Workspace: anySessionProvider{rangeTestDir}, Flags: &stubFilesFlags{rangeRead: true}}, mem)
	t.Cleanup(ops.analysis.Close)
	svc := &FileService{ops: ops, ranges: newRangeLimiter()}

	gate := make(chan struct{})
	mem.SetOnRead(func(n int) {
		if n == 512 { // the sniff read, inside the limiter slot
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
	waitHeld := func(want int) {
		require.Eventually(t, func() bool {
			svc.ranges.perIP.mu.Lock()
			defer svc.ranges.perIP.mu.Unlock()
			return svc.ranges.perIP.total == want
		}, 2e9, 1e6)
	}

	hold("s1", maxRangesPerClient)
	waitHeld(maxRangesPerClient)
	assert.Equal(t, string(rangeBusyPerClient), rangeBusyReason(t, call("s1")), "fifth call from one client")

	hold("s2", maxRangesPerClient)
	waitHeld(maxRangesPerIP)
	assert.Equal(t, string(rangeBusyPerIP), rangeBusyReason(t, call("s3")), "ninth call from one IP")

	close(gate)
	wg.Wait()
	assert.NoError(t, call("s1"), "slots are released")
}

func withSession(r *sessionv1.ReadFileRangeRequest, id string) *sessionv1.ReadFileRangeRequest {
	r.SessionId = id
	return r
}
