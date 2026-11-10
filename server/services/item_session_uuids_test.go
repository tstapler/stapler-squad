package services

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

func TestNonEmptySessionUUIDs(t *testing.T) {
	got := NonEmptySessionUUIDs([]session.ItemSessionSummary{{SessionUUID: "a"}, {SessionUUID: ""}, {SessionUUID: "b"}})
	require.Equal(t, []string{"a", "b"}, got)
	require.Empty(t, NonEmptySessionUUIDs(nil))
}
