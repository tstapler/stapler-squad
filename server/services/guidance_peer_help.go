package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// ErrNotPeerHelp is returned when a peer-help action targets a request of
// another source.
var ErrNotPeerHelp = errors.New("not a peer help request")

// CancelPeerHelp withdraws an open peer-help request on the operator's behalf
// and wakes the asker through its guidance_answered trigger so it stops
// waiting ("cancelled"). applied is false when the request was already
// answered or cancelled; data then holds the current state.
func (s *GuidanceRequestService) CancelPeerHelp(ctx context.Context, id string) (data *session.GuidanceRequestData, applied bool, err error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, false, fmt.Errorf("invalid guidance request id: %w", err)
	}
	data, err = s.storage.GetGuidanceRequest(ctx, parsed)
	if err != nil {
		return nil, false, err
	}
	if data.Source != domain.QuestionSourcePeer {
		return data, false, ErrNotPeerHelp
	}
	applied, err = s.storage.CancelGuidanceRequest(ctx, parsed, wakes.OperatorCancelReason)
	if err != nil {
		return nil, false, err
	}
	if data, err = s.storage.GetGuidanceRequest(ctx, parsed); err != nil {
		return nil, false, err
	}
	if applied {
		s.notifyAnswerListener(ctx, data.ID)
	}
	return data, applied, nil
}
