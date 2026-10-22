package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
)

// ReadFileRange is a read RPC: reads are not guarded today, and this asserts the
// decision so adding it to guardedProcedures later is a deliberate change.
func TestReadFileRange_ShouldStayOutOfGuardedProcedures(t *testing.T) {
	_, guarded := guardedProcedures["/api"+sessionv1connect.SessionServiceReadFileRangeProcedure]
	assert.False(t, guarded)
}
