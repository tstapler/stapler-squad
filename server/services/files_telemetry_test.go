package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/server/services/fsops"
)

func TestFilesTelemetry_ShouldDefaultToNoop_When_NoProvider(t *testing.T) {
	tel := fsops.NewFilesTelemetry(nil)
	assert.NotPanics(t, func() {
		tel.RecordWrite(context.Background(), "s", fsops.WriteOutcomeEvent{})
		tel.RecordSearch(context.Background(), "s", fsops.SearchOutcomeEvent{})
	})
	assert.NotPanics(t, func() {
		NewFileOps(FilePolicy{}, fsops.OsRootBackend{}) // services construct without an analytics provider
	})
}
