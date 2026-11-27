package qareply

import (
	"time"

	"connectrpc.com/connect"

	"github.com/tstapler/stapler-squad/server/services/auditkit"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
	"github.com/tstapler/stapler-squad/session/instance"
)

// Ports is what the Reply path and the answer router need from their owner.
// FindInstance, AuditSink, Count, Canary and InstalledMirror are required; the
// rest are seams whose nil value is the production behaviour.
type Ports struct {
	// FindInstance returns the live session with this title or UUID, or nil.
	FindInstance func(id string) *instance.Instance
	// AuditSink returns the audit sink; a nil sink refuses every write.
	AuditSink func() *auditkit.AuditSink
	// Count adds one reply outcome to the metrics.
	Count func(label string)
	// Canary returns the dialog-drift canary, or nil before it is installed.
	Canary func() *questionkit.DialogMatchCanary
	// InstalledMirror returns the hook-question mirror, or nil before it is installed.
	InstalledMirror func() *questionkit.GuidanceHookMirror

	// Now is the clock; nil is time.Now.
	Now func() time.Time
	// PeerAddr reads a request's peer; nil is req.Peer().Addr until the answer
	// router installs the in-process one.
	PeerAddr func(connect.AnyRequest) string
	// ControllerActive reports a watched pane; nil is "a status controller is started".
	ControllerActive func(*instance.Instance) bool
	// After is the timer behind the send timeout and the closed-check polls; nil is time.After.
	After func(time.Duration) <-chan time.Time
}

// Service is the Reply machinery of one server: pending questions, reply-id
// idempotency, limiters and the flag.
type Service struct {
	ports    Ports
	store    *questionkit.PendingQuestionStore
	flights  *questionkit.ReplyFlightTable
	flag     *ReplyFlag
	limiters *replyLimiters
	peerAddr func(connect.AnyRequest) string
}

// New builds the Reply machinery with the flag off.
func New(p Ports) *Service {
	now := p.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		ports:    p,
		store:    questionkit.NewPendingQuestionStore(now),
		flights:  questionkit.NewReplyFlightTable(now),
		flag:     &ReplyFlag{},
		limiters: newReplyLimiters(now),
		peerAddr: p.PeerAddr,
	}
}

// PendingQuestions is the registry the approval handler feeds.
func (s *Service) PendingQuestions() *questionkit.PendingQuestionStore { return s.store }

// SessionDeleted drops everything Reply keeps for a deleted session.
func (s *Service) SessionDeleted(sessionUUID string) {
	s.store.SessionDeleted(sessionUUID)
	s.limiters.forget(sessionUUID)
}

// controllerActiveFor reports whether a status controller runs for inst. The
// status value is never consulted: only that the pane is being watched.
func (s *Service) controllerActiveFor(inst *instance.Instance) bool {
	if s.ports.ControllerActive != nil {
		return s.ports.ControllerActive(inst)
	}
	c := inst.GetController()
	return c != nil && c.IsStarted()
}
