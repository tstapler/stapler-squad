package flow

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// Span builders shared by the dwell/funnel/rework/lead-time/PR-fix tests.

var s12Base = time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)

func s12Real(t *testing.T, item string, from, to domain.BacklogStatus, at time.Time) RealSpan {
	t.Helper()
	rs, err := NewRealSpan(ItemID(item), from, to, at)
	require.NoError(t, err)
	return rs
}

func s12RealNote(t *testing.T, item string, from, to domain.BacklogStatus, at time.Time, note string) RealSpan {
	t.Helper()
	return s12Real(t, item, from, to, at).WithNote(note)
}

func s12Thrash(t *testing.T, item string, pair FlipPair, first, last time.Time, net domain.BacklogStatus) ThrashSpan {
	t.Helper()
	ts, err := NewThrashSpan(DefaultConfig(), ThrashSpanParams{
		Item: ItemID(item), Pair: pair, Count: 6, First: first, Last: last, NetStatus: net, Cause: CausePRFixReopen,
	})
	require.NoError(t, err)
	return ts
}

func s12Item(n int) string { return fmt.Sprintf("item-%03d", n) }

// s12Merged returns the minimal real path to a first merge at mergeAt.
func s12Merged(t *testing.T, item string, mergeAt time.Time) []Span {
	t.Helper()
	return []Span{
		s12Real(t, item, stReady, stProg, mergeAt.Add(-72*time.Hour)),
		s12Real(t, item, stProg, stReview, mergeAt.Add(-48*time.Hour)),
		s12Real(t, item, stReview, stPRPend, mergeAt.Add(-24*time.Hour)),
		s12Real(t, item, stPRPend, stDoneStat, mergeAt),
	}
}
