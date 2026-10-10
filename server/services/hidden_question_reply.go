package services

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// ReplyToPendingQuestion answers one outstanding single-select AskUserQuestion
// in a hidden session with one option digit (ADR-010).
// +api: session:reply-to-question
func (s *SessionService) ReplyToPendingQuestion(
	ctx context.Context,
	req *connect.Request[sessionv1.ReplyToPendingQuestionRequest],
) (*connect.Response[sessionv1.ReplyToPendingQuestionResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("ReplyToPendingQuestion is not implemented yet"))
}
