package services

import (
	"context"
	"strconv"
	"sync"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/internal/hookipc"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// StopPayload is the subset of the Claude Stop hook body the server reads.
type StopPayload struct {
	SessionID            string `json:"session_id"`
	TranscriptPath       string `json:"transcript_path"`
	PromptID             string `json:"prompt_id"`
	LastAssistantMessage string `json:"last_assistant_message"`
	StopHookActive       bool   `json:"stop_hook_active"`

	// ReceivedAt is stamped by the receiver; the dedup key falls back to it.
	ReceivedAt time.Time `json:"-"`
}

// StopObserver is told about each Stop hook; sessionID is the X-CS-Session-ID header
// (the session title).
type StopObserver func(ctx context.Context, sessionID string, body StopPayload)

// stopDedupKey is session title + the turn's identity: the prompt id when the payload has
// one, else the Stop timestamp.
func stopDedupKey(title string, body StopPayload) string {
	id := body.PromptID
	if id == "" {
		id = "t" + strconv.FormatInt(body.ReceivedAt.UnixNano(), 10)
	}
	return title + "\x00" + id
}

// turnEndTarget is the slice of *session.Instance a Stop needs. It deliberately has no
// Start/RecoverFromStopped: a Stop only ever records a fact, even for an archived session.
type turnEndTarget interface {
	GetProgram() string
	IsArchived() bool
	RecordClassifiedTurnEnd(text string, source session.TurnEndSource, kind session.TurnEndKind, conf session.TurnEndConfidence)
}

// stopTurnEnds turns Stop hooks into recorded TurnEnded facts. Its zero value is not usable;
// build it with newStopTurnEnds.
type stopTurnEnds struct {
	resolve     func(sessionID string) turnEndTarget // nil when unknown
	notify      func(ctx context.Context, sessionID, title, message string)
	projectsDir func() (string, error)
	dedup       *HookDeduper // reuse of the permission-hook coalescer: key seen => skip
	unknownSeen sync.Map
}

func newStopTurnEnds(resolve func(string) turnEndTarget, notify func(context.Context, string, string, string)) *stopTurnEnds {
	return &stopTurnEnds{resolve: resolve, notify: notify, projectsDir: session.ClaudeProjectsDir, dedup: NewHookDeduper()}
}

func (h *stopTurnEnds) handle(ctx context.Context, sessionID string, body StopPayload) {
	target := h.resolve(sessionID)
	if target == nil {
		if _, logged := h.unknownSeen.LoadOrStore(sessionID, struct{}{}); !logged {
			log.Info("[hook/stop] unknown session; no turn end recorded", "session", sessionID)
		}
		return
	}
	// A duplicate delivery of the same Stop returns the cached reply without re-running record.
	_, _ = h.dedup.Do(ctx, hookipc.HookRequestID(stopDedupKey(sessionID, body)), true,
		func() (hookipc.ClassificationReply, error) {
			h.record(ctx, sessionID, target, body)
			return hookipc.ClassificationReply{}, nil
		})
}

func (h *stopTurnEnds) record(ctx context.Context, sessionID string, target turnEndTarget, body StopPayload) {
	text, kind, conf := h.messageFor(sessionID, body)
	if kind != session.TurnEndKindUnknown {
		class := session.ClassifyTurnEnd(target.GetProgram(), text)
		kind, conf = session.TurnEndKindFinished, class.Confidence
		if class.Class == session.TurnEndClassQuestion {
			kind = session.TurnEndKindQuestion
		}
	}
	target.RecordClassifiedTurnEnd(text, session.TurnEndSourceHook, kind, conf)
	if h.notify != nil && !target.IsArchived() {
		h.notify(ctx, sessionID, "Task Complete", "Task completed")
	}
}

// messageFor prefers the payload's last_assistant_message and falls back to the transcript
// tail. Kind is Unknown (and text empty) when neither yields a message.
func (h *stopTurnEnds) messageFor(sessionID string, body StopPayload) (string, session.TurnEndKind, session.TurnEndConfidence) {
	if body.LastAssistantMessage != "" {
		return body.LastAssistantMessage, session.TurnEndKindFinished, session.TurnEndConfidenceLow
	}
	unknown := func() (string, session.TurnEndKind, session.TurnEndConfidence) {
		return "", session.TurnEndKindUnknown, session.TurnEndConfidenceLow
	}
	if body.TranscriptPath == "" {
		return unknown()
	}
	dir, err := h.projectsDir()
	if err != nil {
		log.Warn("[hook/stop] claude projects dir unavailable", "session", sessionID, "err", err)
		return unknown()
	}
	msg, found, err := session.ReadLastAssistantMessage(dir, body.TranscriptPath)
	if err != nil || !found {
		if err != nil {
			log.Warn("[hook/stop] transcript tail unreadable", "session", sessionID, "err", err)
		}
		return unknown()
	}
	return msg.Text, session.TurnEndKindFinished, session.TurnEndConfidenceLow
}

// ObserveStop is the StopObserver wired in server.go: it resolves the hook's session title
// through the review-queue poller and records the turn end.
func (s *SessionService) ObserveStop(ctx context.Context, sessionID string, body StopPayload) {
	s.stopTurnEndsOnce.Do(func() {
		s.stopTurns = newStopTurnEnds(
			func(id string) turnEndTarget {
				if inst := s.findInstance(id); inst != nil {
					return inst
				}
				return nil
			},
			s.notifyStopEndpoint,
		)
	})
	s.stopTurns.handle(ctx, sessionID, body)
}

// notifyStopEndpoint raises the Task Complete notification the script's stop branch no
// longer emits once the endpoint is on.
func (s *SessionService) notifyStopEndpoint(ctx context.Context, sessionID, title, message string) {
	if s.notificationSvc == nil {
		return
	}
	err := s.notificationSvc.SendInProcess(ctx, &sessionv1.SendNotificationRequest{
		SessionId:        sessionID,
		NotificationType: sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE,
		Priority:         sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_LOW,
		Title:            title,
		Message:          message,
	})
	if err != nil {
		log.Warn("[hook/stop] notification not sent", "session", sessionID, "err", err)
	}
}
