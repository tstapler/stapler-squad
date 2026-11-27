package notifyactions

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/auditkit"
	"github.com/tstapler/stapler-squad/server/services/guidance"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

func TestDescriptorSources_ShouldOfferStorageLookupsOnlyWithStorage(t *testing.T) {
	t.Parallel()
	bare := Deps{}.DescriptorSources()
	assert.NotNil(t, bare.Approvals)
	assert.Nil(t, bare.Backlog)
	assert.Nil(t, bare.Guidance)

	stored := Deps{Storage: svctest.CreateTestStorage(t)}.DescriptorSources()
	assert.NotNil(t, stored.Backlog)
	assert.NotNil(t, stored.Guidance)
}

func TestNewDispatcher_ShouldLeaveOptionalDelegatesNil_WhenTheirInputsAreMissing(t *testing.T) {
	t.Parallel()
	d := NewDispatcher(Deps{Registry: testFlagRegistry()})
	assert.NotNil(t, d.approvals)
	assert.NotNil(t, d.resolver)
	assert.Nil(t, d.guidance)
	assert.Nil(t, d.backlog)
	assert.Nil(t, d.cancels)

	storage := svctest.CreateTestStorage(t)
	assert.Nil(t, NewDispatcher(Deps{Registry: testFlagRegistry(), Storage: storage}).backlog, "no transitioner")
	assert.Nil(t, NewDispatcher(Deps{Registry: testFlagRegistry(), Transitions: &itemWorld{}}).backlog, "no storage")
}

func TestNewDispatcher_ShouldWireOptionalDelegates_WhenTheirInputsArePresent(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	world := &itemWorld{}
	d := NewDispatcher(Deps{
		Registry:    testFlagRegistry(),
		Storage:     storage,
		Transitions: world,
		Merger:      world,
		Guidance:    guidance.NewGuidanceRequestService(storage, nil, nil),
	})
	assert.NotNil(t, d.guidance)
	assert.NotNil(t, d.backlog)
}

// A nil *AuditSink must reach the dispatcher as a nil interface, so Tier B
// refuses instead of calling a nil sink.
func TestNewDispatcher_ShouldTreatAMissingSinkAsNoAudit(t *testing.T) {
	t.Parallel()
	for name, audit := range map[string]func() *auditkit.AuditSink{
		"no func":  nil,
		"nil sink": func() *auditkit.AuditSink { return nil },
	} {
		d := NewDispatcher(Deps{Registry: testFlagRegistry(), Audit: audit})
		assert.Nil(t, d.sink(), name)
		assert.ErrorIs(t, d.appendRequest(context.Background(), auditkit.AuditLine{}), auditkit.ErrAuditFailed, name)
		d.appendResultLine(auditkit.AuditLine{}, sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED)
	}

	sink := &auditkit.AuditSink{}
	d := NewDispatcher(Deps{Registry: testFlagRegistry(), Audit: func() *auditkit.AuditSink { return sink }})
	assert.Same(t, sink, d.sink())
}
