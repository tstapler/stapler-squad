package conditional

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestCheck_FirstCallIsMissAndCarriesEtag(t *testing.T) {
	ResetCountsForTest()
	body := wrapperspb.String("hello")

	etag, notModified := Check("RpcA", "", body)

	assert.NotEmpty(t, etag)
	assert.False(t, notModified, "no validator sent: must serve the full body")
	assert.Equal(t, int64(1), Count("RpcA", OutcomeMiss))
}

func TestCheck_SameContentWithEtagIsNotModified(t *testing.T) {
	ResetCountsForTest()
	body := wrapperspb.String("hello")
	etag, _ := Check("RpcA", "", body)

	etag2, notModified := Check("RpcA", etag, wrapperspb.String("hello"))

	assert.True(t, notModified)
	assert.Equal(t, etag, etag2)
	assert.Equal(t, int64(1), Count("RpcA", OutcomeNotModified))
}

func TestCheck_ChangedContentInvalidates(t *testing.T) {
	ResetCountsForTest()
	etag, _ := Check("RpcA", "", wrapperspb.String("v1"))

	etag2, notModified := Check("RpcA", etag, wrapperspb.String("v2"))

	assert.False(t, notModified, "a write must be visible on the next poll")
	assert.NotEqual(t, etag, etag2)
}

func TestCheck_EtagIsBoundToRpc(t *testing.T) {
	etagA, _ := Check("RpcA", "", wrapperspb.String("same"))

	_, notModified := Check("RpcB", etagA, wrapperspb.String("same"))

	assert.False(t, notModified, "a validator minted for one RPC must not validate another")
}

func TestCheck_ClientCannotMintAValidator(t *testing.T) {
	// The tag is keyed by a per-process secret: a client that knows the content
	// (or guesses a hash of it) still cannot produce a matching validator.
	guess := Validator("\"0123456789abcdef0123456789abcdef\"")

	_, notModified := Check("RpcA", guess, wrapperspb.String("hello"))

	assert.False(t, notModified)
}

func TestCheck_EmptyValidatorNeverMatches(t *testing.T) {
	_, notModified := Check("RpcA", "", wrapperspb.String(""))

	assert.False(t, notModified)
}

func TestTagMessage_StableWithinProcess(t *testing.T) {
	a, err := TagMessage("RpcA", wrapperspb.String("x"))
	require.NoError(t, err)
	b, err := TagMessage("RpcA", wrapperspb.String("x"))
	require.NoError(t, err)

	assert.Equal(t, a, b)
}
