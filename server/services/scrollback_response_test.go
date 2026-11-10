package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildScrollbackResponse_ChunkUsesCRLFAndCountsLinesFromRawContent(t *testing.T) {
	t.Parallel()

	data := buildScrollbackResponse("s1", "", "one\ntwo\r\nthree\n", 0, 500)

	resp := data.GetScrollbackResponse()
	require.NotNil(t, resp)
	require.Len(t, resp.Chunks, 1)
	assert.Equal(t, "one\r\ntwo\r\nthree\r\n", string(resp.Chunks[0].Data))
	assert.Equal(t, uint64(3), resp.TotalLines)
	assert.False(t, resp.HasMore)
}

func TestBuildScrollbackResponse_EmptyContentHasNoChunks(t *testing.T) {
	t.Parallel()

	resp := buildScrollbackResponse("s1", "", "\n", 0, 500).GetScrollbackResponse()

	require.NotNil(t, resp)
	assert.Empty(t, resp.Chunks)
}
