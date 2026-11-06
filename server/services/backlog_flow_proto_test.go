package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// flowNumberFields are the field names that make a message a "metric": a number
// a reader may want to open. Each such message must carry drill_key beside it.
var flowNumberFields = map[protoreflect.Name]bool{"value": true, "count": true, "ratio": true}

func isNumericKind(k protoreflect.Kind) bool {
	switch k {
	case protoreflect.BoolKind, protoreflect.StringKind, protoreflect.BytesKind,
		protoreflect.MessageKind, protoreflect.GroupKind, protoreflect.EnumKind:
		return false
	}
	return true
}

func TestBacklogFlowProto_should_CarryDrillKey_When_MessageHasNumericValueCountOrRatio(t *testing.T) {
	t.Parallel()
	msgs := sessionv1.File_session_v1_backlog_flow_proto.Messages()
	require.Positive(t, msgs.Len())
	checked := 0
	for i := 0; i < msgs.Len(); i++ {
		md := msgs.Get(i)
		fields := md.Fields()
		hasNumber := false
		for j := 0; j < fields.Len(); j++ {
			f := fields.Get(j)
			if flowNumberFields[f.Name()] && isNumericKind(f.Kind()) && !f.IsList() {
				hasNumber = true
			}
		}
		if !hasNumber {
			continue
		}
		checked++
		dk := fields.ByName("drill_key")
		if assert.NotNil(t, dk, "message %s has a numeric value/count/ratio but no drill_key", md.Name()) {
			assert.Equal(t, protoreflect.StringKind, dk.Kind(), "%s.drill_key must be a string", md.Name())
		}
	}
	assert.GreaterOrEqual(t, checked, 4, "walk found too few metric messages; the detector is broken")
}

func TestBacklogFlowProto_should_ExposeSixRPCs(t *testing.T) {
	t.Parallel()
	svc := sessionv1.File_session_v1_backlog_flow_proto.Services().ByName("BacklogFlowService")
	require.NotNil(t, svc)
	got := map[string]bool{}
	for i := 0; i < svc.Methods().Len(); i++ {
		got[string(svc.Methods().Get(i).Name())] = true
	}
	for _, want := range []string{"GetFlowSummary", "ListDeadlockCandidates", "ListThrashEpisodes", "GetItemFlowTimeline", "CompareFlowWindows", "ListFlowDrilldown"} {
		assert.True(t, got[want], want)
	}
	assert.Len(t, got, 6)
}

func TestBacklogFlowProto_should_HaveNoAllTimeOnCompareAndEstimatedLegendOnMeta(t *testing.T) {
	t.Parallel()
	req := (&sessionv1.CompareFlowWindowsRequest{}).ProtoReflect().Descriptor().Fields()
	assert.Nil(t, req.ByName("all_time"))
	for _, n := range []protoreflect.Name{"a_days", "b_days", "b_offset_days", "a_start", "a_end", "b_start", "b_end"} {
		assert.NotNil(t, req.ByName(n), string(n))
	}
	legend := (&sessionv1.FlowMeta{}).ProtoReflect().Descriptor().Fields().ByName("estimated_legend")
	require.NotNil(t, legend)
	assert.True(t, legend.IsMap())
	sum := (&sessionv1.GetFlowSummaryRequest{}).ProtoReflect().Descriptor().Fields()
	for _, n := range []protoreflect.Name{"window_days", "all_time", "sections", "compact"} {
		assert.NotNil(t, sum.ByName(n), string(n))
	}
}
