package panereply

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubmitReplySequence_ShouldStopBeforeTheRefusedKey_WhenBeforeEachWriteFails(t *testing.T) {
	t.Parallel()
	refused := errors.New("refused")
	for k := range len(recordedKeys(t)) {
		sim := newSim()
		_, lease := replyInstanceAndLease(t)
		calls := 0
		opts := seqOpts()
		opts.BeforeEachWrite = func() error {
			if calls++; calls > k {
				return refused
			}
			return nil
		}

		res := SubmitReplySequence(context.Background(), sim, lease, recordedSteps(t), opts)

		require.ErrorIs(t, res.Err, refused)
		assert.Equal(t, k, res.Written)
		assert.Len(t, sim.written(), k)
		want := StageWriteEntered
		if k == 0 {
			want = StageNotStarted
		}
		assert.Equal(t, want, res.Stage)
	}
}
