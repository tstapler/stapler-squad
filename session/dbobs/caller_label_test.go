package dbobs

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCallerLabel_should_KeepLeafAndBoundCardinality(t *testing.T) {
	// Not parallel: mutates the package-level seen set.
	callerLabelMu.Lock()
	callerLabelSeen = map[string]struct{}{}
	callerLabelMu.Unlock()

	assert.Equal(t, "session.EntRepository.ListItemSessions", callerLabel("session.EntRepository.ListItemSessions<-session.Storage.ListItemSessions<-x"))
	assert.Equal(t, "unknown", callerLabel(""))
	for i := 0; i < maxCallerLabels+10; i++ {
		callerLabel(fmt.Sprintf("pkg.Fn%d<-caller", i))
	}
	assert.Equal(t, callerOverflow, callerLabel("brand.New<-x"), "labels past the cap fold into one bucket")
	assert.Equal(t, "session.EntRepository.ListItemSessions", callerLabel("session.EntRepository.ListItemSessions<-y"), "known labels stay stable after the cap")
}
