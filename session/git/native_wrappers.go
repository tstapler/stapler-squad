package git

import (
	"context"

	"go.opentelemetry.io/otel/attribute"

	"github.com/tstapler/stapler-squad/session/git/native"
)

// Thin wrappers over session/git/native's observability helpers so existing call sites
// in this package keep their original names.

const (
	outcomeSuccess       = native.OutcomeSuccess
	outcomeError         = native.OutcomeError
	implementationNative = native.ImplementationNative
)

func spanOutcome(err error) string { return native.SpanOutcome(err) }

func withOperationAttrs(ctx context.Context, attrs ...attribute.KeyValue) context.Context {
	return native.WithOperationAttrs(ctx, attrs...)
}

func withOperationSpan(ctx context.Context, op string, fn func() (implementation, outcome string, err error)) error {
	return native.WithOperationSpan(ctx, op, fn)
}
