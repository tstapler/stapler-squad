package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/server/services"
)

// A batch dispatches through the listener chain, but a guarded (write-capable) procedure
// must never be offered to it at all.
func TestBatchAllowed_should_ContainNoGuardedProcedure(t *testing.T) {
	for _, m := range services.BatchAllowedMethods() {
		_, guarded := guardedProcedures["/api"+m]
		assert.False(t, guarded, "%s is in guardedProcedures and must not be batchable", m)
	}
}
