package services

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func TestRangeOutcome_ShouldLabelEachFailedPreconditionByCause(t *testing.T) {
	fp := func(cause error) error { return connect.NewError(connect.CodeFailedPrecondition, cause) }
	cases := map[string]error{
		"disabled":      fp(errOpNotServed(sessionv1.FileOp_FILE_OP_READ_LINES)),
		"index_pending": fp(errRangeIndexPending),
		"too_large":     fp(fmt.Errorf("%w (%d bytes)", errRangeTooLarge, int64(2)<<30)),
		"refused":       fp(errors.New("worktree is missing")),
	}
	for want, err := range cases {
		assert.Equal(t, want, rangeOutcome(nil, err), want)
	}
}

func TestInternalIOError_ShouldKeepCauseOffTheWire(t *testing.T) {
	err := internalIOError("read", "dir/f.txt", errors.New("read /home/u/secret/dir/f.txt: input/output error"), "failed to read file")

	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.Equal(t, "internal: failed to read file", err.Error())
}

func TestInternalIOError_ShouldLogOpPathAndCauseServerSide(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	_ = internalIOError("read_range", "dir/f.txt", errors.New("read dir/f.txt: input/output error"), "failed to read file range")
	_ = openRootError(errors.New("open /srv/worktree: permission denied"))

	out := logs.String()
	assert.Contains(t, out, "op=read_range")
	assert.Contains(t, out, "dir/f.txt")
	assert.Contains(t, out, "input/output error")
	assert.Contains(t, out, "op=open_root")
}
