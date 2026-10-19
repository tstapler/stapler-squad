package services

import (
	"errors"
	"fmt"
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
