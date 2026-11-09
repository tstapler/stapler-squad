package mcp

import (
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"strings"
)

const (
	// flowLeafMaxLen caps every string leaf that can reach the agent.
	flowLeafMaxLen = 300
	// flowKeyEchoMaxLen caps a caller-supplied drill key echoed in an error.
	flowKeyEchoMaxLen = 80
)

// flowSafeStringFields are the string fields the server generates from closed
// vocabularies (section status, heuristic codes, compare state, timeline kind,
// rework cause, thrash status pair); every other string leaf is sanitized. The
// thrash pair is allowlisted because its "a<>b" form would lose the "<>" to the
// HTML-tag stripper. A test fails if an allowlisted field stops existing.
var flowSafeStringFields = map[protoreflect.FullName]bool{
	"session.v1.FlowSectionStatus.status":    true,
	"session.v1.FlowSectionStatus.estimated": true,
	"session.v1.FlowMeta.estimated":          true,
	"session.v1.FlowCompareMetric.state":     true,
	"session.v1.FlowTimelineEntry.kind":      true,
	"session.v1.FlowTimelineEntry.cause":     true,
	"session.v1.FlowThrashItemRow.cause":     true,
	"session.v1.FlowThrashItemRow.pair":      true,
}

// sanitizeFlowLeaf flattens control characters and whitespace runs, strips
// HTML tags and truncates, so stored titles, notes and error text cannot
// inject a line break or a long payload into the agent's context.
func sanitizeFlowLeaf(s string) string {
	flat := services.SanitizeFlowText(s, int(^uint(0)>>1))
	return cutAtRuneBoundary(session.SanitizeForAgentContext(flat, flowLeafMaxLen))
}

// cutAtRuneBoundary drops the partial rune that SanitizeForAgentContext's byte
// truncation can leave, which proto marshalling rejects as invalid UTF-8.
func cutAtRuneBoundary(s string) string { return strings.ToValidUTF8(s, "") }

func sanitizeFlowKeyEcho(s string) string {
	flat := services.SanitizeFlowText(s, int(^uint(0)>>1))
	return cutAtRuneBoundary(session.SanitizeForAgentContext(flat, flowKeyEchoMaxLen))
}

// sanitizeFlowMessage walks m by reflection and sanitizes every string leaf
// (including map keys and values and nested or repeated messages) except the
// fields in flowSafeStringFields.
func sanitizeFlowMessage(m proto.Message) {
	if m == nil {
		return
	}
	sanitizeReflect(m.ProtoReflect())
}

func sanitizeReflect(msg protoreflect.Message) {
	msg.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch {
		case fd.IsMap():
			sanitizeMap(msg, fd, v.Map())
		case fd.IsList():
			sanitizeList(fd, v.List())
		case fd.Kind() == protoreflect.MessageKind:
			sanitizeReflect(v.Message())
		case fd.Kind() == protoreflect.StringKind && !flowSafeStringFields[fd.FullName()]:
			msg.Set(fd, protoreflect.ValueOfString(sanitizeFlowLeaf(v.String())))
		}
		return true
	})
}

func sanitizeList(fd protoreflect.FieldDescriptor, list protoreflect.List) {
	safe := flowSafeStringFields[fd.FullName()]
	for i := 0; i < list.Len(); i++ {
		switch fd.Kind() {
		case protoreflect.MessageKind:
			sanitizeReflect(list.Get(i).Message())
		case protoreflect.StringKind:
			if !safe {
				list.Set(i, protoreflect.ValueOfString(sanitizeFlowLeaf(list.Get(i).String())))
			}
		}
	}
}

func sanitizeMap(msg protoreflect.Message, fd protoreflect.FieldDescriptor, m protoreflect.Map) {
	type entry struct {
		k protoreflect.MapKey
		v protoreflect.Value
	}
	var entries []entry
	m.Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
		entries = append(entries, entry{k, v})
		return true
	})
	mapVal := fd.MapValue()
	for _, e := range entries {
		m.Clear(e.k)
		key, val := e.k, e.v
		if fd.MapKey().Kind() == protoreflect.StringKind {
			key = protoreflect.ValueOfString(sanitizeFlowLeaf(e.k.String())).MapKey()
		}
		switch mapVal.Kind() {
		case protoreflect.StringKind:
			val = protoreflect.ValueOfString(sanitizeFlowLeaf(e.v.String()))
		case protoreflect.MessageKind:
			sanitizeReflect(e.v.Message())
		}
		m.Set(key, val)
	}
}
