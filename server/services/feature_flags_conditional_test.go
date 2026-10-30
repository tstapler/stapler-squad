package services

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/conditional"
)

func getFlags(t *testing.T, svc *SessionService, ifNoneMatch string) *sessionv1.GetFeatureFlagsResponse {
	t.Helper()
	resp, err := svc.GetFeatureFlags(context.Background(),
		connect.NewRequest(&sessionv1.GetFeatureFlagsRequest{IfNoneMatch: ifNoneMatch}))
	require.NoError(t, err)
	return resp.Msg
}

// A client that sends no validator (old client) always gets the full body.
func TestGetFeatureFlags_NoValidatorServesFullBody(t *testing.T) {
	svc := newFeatureFlagService(t)

	first := getFlags(t, svc, "")
	second := getFlags(t, svc, "")

	assert.NotEmpty(t, first.Flags)
	assert.NotEmpty(t, first.Etag)
	assert.False(t, first.NotModified)
	assert.Len(t, second.Flags, len(first.Flags))
	assert.False(t, second.NotModified)
}

func TestGetFeatureFlags_UnchangedPollIsNotModified(t *testing.T) {
	conditional.ResetCountsForTest()
	svc := newFeatureFlagService(t)
	first := getFlags(t, svc, "")

	again := getFlags(t, svc, first.Etag)

	assert.True(t, again.NotModified)
	assert.Empty(t, again.Flags, "an elided body carries no flags")
	assert.Equal(t, first.Etag, again.Etag)
	assert.Equal(t, int64(1), conditional.Count("GetFeatureFlags", conditional.OutcomeNotModified))
}

// Read-your-writes: a toggle must be visible on the very next poll, never a 304.
func TestGetFeatureFlags_WriteInvalidatesValidator(t *testing.T) {
	svc := newFeatureFlagService(t)
	before := getFlags(t, svc, "")

	_, err := svc.UpdateFeatureFlag(context.Background(), connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{
		Name: "backlog", Enabled: true,
	}))
	require.NoError(t, err)
	after := getFlags(t, svc, before.Etag)

	assert.False(t, after.NotModified)
	assert.NotEqual(t, before.Etag, after.Etag)
	var enabled bool
	for _, f := range after.Flags {
		if f.Name == "backlog" {
			enabled = f.Enabled
		}
	}
	assert.True(t, enabled)
}

// A validator an attacker invented, or one minted for another RPC, never elides the body.
func TestGetFeatureFlags_ForeignValidatorServesFullBody(t *testing.T) {
	svc := newFeatureFlagService(t)

	got := getFlags(t, svc, "\"AAAAAAAAAAAAAAAAAAAAAA\"")

	assert.False(t, got.NotModified)
	assert.NotEmpty(t, got.Flags)
}
