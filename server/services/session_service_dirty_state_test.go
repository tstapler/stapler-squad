package services

import (
	"errors"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/session"
)

func TestClassifyErr_should_ReturnFailedPrecondition_When_DirtyStateUnknown(t *testing.T) {
	t.Parallel()
	err := fmt.Errorf("%w: %w", session.ErrDirtyStateUnknown, errors.New("status failed"))

	assert.Equal(t, connect.CodeFailedPrecondition, classifyStopErr(err, "stop").Code())
	assert.Equal(t, connect.CodeFailedPrecondition, classifyPauseResumeErr(err, "pause").Code())
}
