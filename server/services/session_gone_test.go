package services

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionGone_NotFoundMapsToNotFoundEndStreamCode(t *testing.T) {
	cause := fmt.Errorf("tymux: CapturePane: %w", connect.NewError(connect.CodeNotFound, errors.New("no such pane")))
	require.True(t, isSessionGone(cause))

	wrapped := wrapSessionGone(cause)
	assert.ErrorIs(t, wrapped, errSessionGone)
	assert.Equal(t, "not_found", endStreamErrorCode(wrapped))
}

func TestSessionGone_OtherErrorsUnchanged(t *testing.T) {
	other := connect.NewError(connect.CodeInternal, errors.New("boom"))
	assert.False(t, isSessionGone(other))
	assert.Same(t, other, wrapSessionGone(other))
	assert.Equal(t, "internal", endStreamErrorCode(other))
	assert.False(t, isSessionGone(nil))
}

func TestLogThrottle_AdmitsOncePerIntervalPerKey(t *testing.T) {
	th := newLogThrottle(30 * time.Second)
	t0 := time.Unix(1000, 0)
	assert.True(t, th.allow("a", t0))
	assert.False(t, th.allow("a", t0.Add(29*time.Second)))
	assert.True(t, th.allow("b", t0.Add(time.Second)), "keys are independent")
	assert.True(t, th.allow("a", t0.Add(30*time.Second)))
}
