package backlogsvc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/domain"
)

func TestTransitionDenialAlreadyOpen_ReadError_DoesNotSilence(t *testing.T) {
	svc := NewBacklogService(svctest.CreateTestStorage(t), &mockSessionCreator{}, nil, nil, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.False(t, svc.transitionDenialAlreadyOpen(ctx, "item", domain.TransitionDenial{Caller: "c", From: "a", To: "b", Reason: "r"}))
}
