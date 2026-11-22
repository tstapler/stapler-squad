package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// With no session service wired (the nil interface the composition root passes), the program
// lookup must be skipped rather than dereferenced, and create_session_for_pr must answer with
// its "unavailable" error.
func TestCreateSessionForPR_should_ReportUnavailable_When_NoSessionServiceIsWired(t *testing.T) {
	t.Parallel()
	gh := &githubHandlers{cache: &fakeUserPRSource{}, store: &stubStore{}, svc: nil}

	res, err := gh.createSessionForPR(context.Background(), makeToolReq(createSessionForPRArgs(t, "pr-no-svc", 21, nil)))
	require.NoError(t, err)

	assert.Equal(t, ErrInternalError, resultErrCode(t, res))
}
