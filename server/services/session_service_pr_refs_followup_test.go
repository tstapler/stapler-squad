package services

import (
	"errors"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/session"
)

func TestPRSetError_ShouldMapCapAndZeroRefToClientErrors(t *testing.T) {
	t.Parallel()
	assert.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(prSetError(fmt.Errorf("attach: %w", session.ErrSessionPRLinkCap))))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(prSetError(session.ErrZeroPRRef)))
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(prSetError(errors.New("boom"))))
}
