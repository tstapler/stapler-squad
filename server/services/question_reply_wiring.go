package services

import (
	"context"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/services/qareply"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
)

// question_reply_wiring.go binds the Reply path and answer router to this
// service's live sessions, audit sink, metrics, canary and hook mirror.

// replyPorts reads every collaborator at call time, so one installed after the
// service was built is still seen.
func (s *SessionService) replyPorts() qareply.Ports {
	return qareply.Ports{
		FindInstance:    s.findInstance,
		AuditSink:       s.auditSink,
		Count:           s.countReply,
		Canary:          s.dialogCanary.Load,
		InstalledMirror: s.guidanceMirror.Load,
	}
}

// replyService is the Reply machinery of this service, built on first use.
func (s *SessionService) replyService() *qareply.Service {
	s.replyOnce.Do(func() { s.reply = qareply.New(s.replyPorts()) })
	return s.reply
}

// wireReplyFlag connects hidden_session_reply to the Reply path.
func (s *SessionService) wireReplyFlag() { s.replyService().WireFlag(s.featureFlagSvc) }

// PendingQuestions is the registry the approval handler feeds.
func (s *SessionService) PendingQuestions() *questionkit.PendingQuestionStore {
	return s.replyService().PendingQuestions()
}

// ReplyToPendingQuestion answers one outstanding single-select AskUserQuestion
// in a hidden session with one option digit (ADR-010).
// +api: session:reply-to-question
func (s *SessionService) ReplyToPendingQuestion(
	ctx context.Context,
	req *connect.Request[sessionv1.ReplyToPendingQuestionRequest],
) (*connect.Response[sessionv1.ReplyToPendingQuestionResponse], error) {
	return s.replyService().Reply(ctx, req)
}

func (s *SessionService) countReply(label string) {
	if s.deliveryGate != nil {
		s.deliveryGate.Metrics().Add(deliverygate.CounterReply, label)
	}
}
