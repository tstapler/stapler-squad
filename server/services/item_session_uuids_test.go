package services

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

func TestNonEmptySessionUUIDs(t *testing.T) {
	got := NonEmptySessionUUIDs([]backlogdto.ItemSessionSummary{{SessionUUID: "a"}, {SessionUUID: ""}, {SessionUUID: "b"}})
	require.Equal(t, []string{"a", "b"}, got)
	require.Empty(t, NonEmptySessionUUIDs(nil))
}
