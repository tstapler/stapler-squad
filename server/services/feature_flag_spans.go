package services

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/tstapler/stapler-squad/telemetry"
)

// flagPhase names a step of UpdateFeatureFlag that gets its own child span.
type flagPhase string

const (
	flagPhaseAudit   flagPhase = "audit_request"
	flagPhasePersist flagPhase = "persist"
	flagPhaseApply   flagPhase = "apply_controller"
)

// flagSpan opens a child span of the UpdateFeatureFlag RPC span so a slow flip
// shows which phase (audit append, config.json persist, controller apply) held it.
func flagSpan(ctx context.Context, op flagOp, phase flagPhase) (end func()) {
	_, span := telemetry.GetTracer().Start(ctx, "featureflag."+string(phase),
		trace.WithAttributes(attribute.String("feature", op.name), attribute.String("mutation", op.mutationName())))
	return func() { span.End() }
}
