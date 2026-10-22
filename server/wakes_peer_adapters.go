package server

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session"
)

// wakePeerRequests backs the PeerHelpEscalator with the guidance table. The
// notified_at column is the durable "operator was told" marker.
type wakePeerRequests struct {
	storage *session.Storage
	titles  func(sessionUUID string) string
}

func (p wakePeerRequests) OpenPeerRequests(ctx context.Context) ([]wakes.PeerRequest, error) {
	rows, err := p.storage.ListOpenPeerGuidanceRequests(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]wakes.PeerRequest, 0, len(rows))
	for _, r := range rows {
		if r.NotifiedAt != nil {
			continue // the operator was already told, before this process started
		}
		out = append(out, wakes.PeerRequest{
			ID: r.ID, Asker: r.AskerSessionUUID, Answerer: r.AnswererSessionUUID, Question: r.QuestionText,
			CreatedAt: r.CreatedAt, AskerTitle: p.titles(r.AskerSessionUUID), AnswererTitle: p.titles(r.AnswererSessionUUID),
		})
	}
	return out, nil
}

func (p wakePeerRequests) MarkEscalated(ctx context.Context, id string) (bool, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return false, err
	}
	return p.storage.MarkGuidanceRequestNotified(ctx, parsed)
}

// sessionTitles names sessions for operator-facing text: the live title, else
// the stored one, else the uuid.
type sessionTitles struct {
	svc     *services.SessionService
	storage *session.Storage
}

func (t sessionTitles) Title(sessionUUID string) string {
	if t.svc != nil {
		if inst := t.svc.FindLiveInstance(sessionUUID); inst != nil {
			if title := inst.Snapshot().Title; title != "" {
				return title
			}
		}
	}
	data, err := t.storage.FindInstanceDataByID(sessionUUID)
	if err != nil && !errors.Is(err, session.ErrInstanceDataNotFound) {
		return sessionUUID
	}
	if err == nil && data.Title != "" {
		return data.Title
	}
	return sessionUUID
}
