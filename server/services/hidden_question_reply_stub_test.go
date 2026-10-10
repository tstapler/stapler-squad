package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// Task 1.7a: the contract PR ships the RPC as a stub.
func TestReplyToPendingQuestion_ShouldReturnUnimplemented_WhenOnlyTheContractIsMerged(t *testing.T) {
	var s SessionService
	_, err := s.ReplyToPendingQuestion(context.Background(), connect.NewRequest(&sessionv1.ReplyToPendingQuestionRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}
