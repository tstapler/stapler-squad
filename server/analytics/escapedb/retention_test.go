package escapedb

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/ent/escapeevent"
)

func TestRetention_EscapeEventPerSessionCapKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	client := openForTest(ctx, t, dir)
	defer client.Close()

	base := time.Now()
	for _, sessionID := range []string{"one", "two"} {
		for i := 0; i < 5; i++ {
			_, err := client.EscapeEvent.Create().
				SetID(sessionID + string(rune('a'+i))).SetSessionID(sessionID).
				SetStage("pty_read").SetSequenceType("SGR").SetByteLength(4).
				SetWallTime(base.Add(time.Duration(i) * time.Second)).SetSessionSeq(int64(i)).Save(ctx)
			require.NoError(t, err)
		}
	}

	RunRetention(ctx, client, 0, 3)
	for _, sessionID := range []string{"one", "two"} {
		events, err := client.EscapeEvent.Query().Where(escapeevent.SessionID(sessionID)).Order(escapeevent.ByWallTime()).All(ctx)
		require.NoError(t, err)
		require.Len(t, events, 3)
		assert.Equal(t, int64(2), events[0].SessionSeq)
	}
}

// Multi-chunk eviction across sessions: over-cap sessions land exactly on the cap
// with the newest rows, an under-cap session is untouched.
func TestRetention_EscapeEventPerSessionCap_MultiChunkAndUnderCap(t *testing.T) {
	ctx := context.Background()
	client := openForTest(ctx, t, t.TempDir())
	defer client.Close()

	const cap = 40
	w := NewEscapeEventBatchWriter(client, 0) // cap 0: insert without trimming
	base := time.Now().Add(-time.Hour)
	sizes := map[string]int{"big": 2*escapeEvictChunkRows + 100 + cap, "mid": cap + 3, "small": cap - 5}
	for s, n := range sizes {
		w.flushBatch(ctx, escapeBatch(s, base, 0, n))
	}

	RunRetention(ctx, client, 0, cap)

	for s, n := range sizes {
		want := min(n, cap)
		got, err := client.EscapeEvent.Query().Where(escapeevent.SessionID(s)).Order(escapeevent.ByWallTime()).All(ctx)
		require.NoError(t, err)
		require.Len(t, got, want, s)
		assert.Equal(t, int64(n-want), got[0].SessionSeq, "%s keeps the newest rows", s)
	}
}
