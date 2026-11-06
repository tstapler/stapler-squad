package session

import (
	"net"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPRProvenance_should_RoundTripItemAndHost_When_FormattedThenParsed(t *testing.T) {
	id, err := NewHostID()
	require.NoError(t, err)
	link := "ssq://laptop/backlog/v1/bl_01J7QK0000000000000000000A"

	got, ok := ParsePRProvenanceComment("Thanks for the PR!\n\n" + FormatPRProvenanceComment(id, link))

	require.True(t, ok)
	assert.Equal(t, link, got.ItemDeepLink)
	assert.Equal(t, "bl_01J7QK0000000000000000000A", got.ItemID)
	assert.Equal(t, id.String(), got.HostID.String())
}

func TestParsePRProvenanceComment_should_Reject_When_MarkerOrPartsMissing(t *testing.T) {
	id, err := NewHostID()
	require.NoError(t, err)
	for name, body := range map[string]string{
		"empty":         "",
		"no marker":     "see ssq://laptop/backlog/v1/bl_1 host " + id.String(),
		"bad deep link": prProvenanceMarker + "\nssq://laptop/oops\nHost: " + id.String(),
		"no host id":    prProvenanceMarker + "\nssq://laptop/backlog/v1/bl_1\n",
	} {
		_, ok := ParsePRProvenanceComment(body)
		assert.False(t, ok, name)
	}
}

func TestClaimIndexRecorder_PRProvenanceComment_should_NameHostByOpaqueIDAndLinkOnly(t *testing.T) {
	identity, err := LoadOrCreateHostIdentity(t.TempDir())
	require.NoError(t, err)
	registry, err := NewHostRegistry(t.TempDir(), DefaultHostRegistryTTL)
	require.NoError(t, err)
	index, err := NewClaimIndex(t.TempDir(), registry)
	require.NoError(t, err)
	recorder, err := NewClaimIndexRecorder(identity, index, nil, "laptop", nil)
	require.NoError(t, err)
	item := &BacklogItemData{ID: "bl_01J7QK0000000000000000000A"}

	body, ok := recorder.PRProvenanceComment(item)

	require.True(t, ok)
	assert.Contains(t, body, "ssq://laptop/backlog/v1/bl_01J7QK0000000000000000000A")
	assert.Contains(t, body, identity.ID.String())
	assert.False(t, regexp.MustCompile(`\d+\.\d+\.\d+\.\d+|:\d{2,5}\b`).MatchString(body), "no IP or host:port in %q", body)
	for _, token := range regexp.MustCompile(`\S+`).FindAllString(body, -1) {
		assert.Nil(t, net.ParseIP(token), "token %q must not be an IP", token)
	}

	noHost, err := NewClaimIndexRecorder(identity, index, nil, "", nil)
	require.NoError(t, err)
	_, ok = noHost.PRProvenanceComment(item)
	assert.False(t, ok, "without a deep-link host there is nothing safe to stamp")
}

func TestStorage_PRProvenanceComment_should_ReportFalse_When_NoSourceWired(t *testing.T) {
	var s Storage
	_, ok := s.PRProvenanceComment(&BacklogItemData{ID: "x"})
	assert.False(t, ok)
}
