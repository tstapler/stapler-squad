package native

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/git/internal/obstest"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// TestWithOperationSpan_RecordsImplementationAndSuccessOutcome is Task 4.4.1a/validation.md's
// P4 happy-path test: a successful op must produce a span named op with implementation/
// outcome attributes matching what fn reported, and no error status.
func TestWithOperationSpan_RecordsImplementationAndSuccessOutcome(t *testing.T) {
	recorder := obstest.InstallProviders(t)

	err := WithOperationSpan(context.Background(), "git.worktree.add", func() (string, string, error) {
		return "native", OutcomeSuccess, nil
	})
	require.NoError(t, err)

	ended := recorder.Ended()
	require.Len(t, ended, 1, "expected exactly one span to have ended")

	span := ended[0]
	assert.Equal(t, "git.worktree.add", span.Name())

	implAttr, ok := obstest.FindSpanAttr(span.Attributes(), "implementation")
	require.True(t, ok, "expected an implementation attribute on the span")
	assert.Equal(t, "native", implAttr.Value.AsString())

	outcomeAttr, ok := obstest.FindSpanAttr(span.Attributes(), "outcome")
	require.True(t, ok, "expected an outcome attribute on the span")
	assert.Equal(t, OutcomeSuccess, outcomeAttr.Value.AsString())

	assert.NotEqual(t, codes.Error, span.Status().Code, "a successful op must not mark the span errored")
}

// TestWithOperationSpan_should_RecordErrorOutcome_When_UnderlyingCallFails is
// validation.md's P4 error-path test: fn returning a non-nil error must mark the span
// errored (RecordError + Error status), not silently "success".
func TestWithOperationSpan_should_RecordErrorOutcome_When_UnderlyingCallFails(t *testing.T) {
	recorder := obstest.InstallProviders(t)

	wantErr := errors.New("boom")
	err := WithOperationSpan(context.Background(), "git.worktree.remove", func() (string, string, error) {
		return "legacy", OutcomeError, wantErr
	})
	require.ErrorIs(t, err, wantErr, "WithOperationSpan must pass fn's error straight through")

	ended := recorder.Ended()
	require.Len(t, ended, 1)

	span := ended[0]
	assert.Equal(t, codes.Error, span.Status().Code)

	outcomeAttr, ok := obstest.FindSpanAttr(span.Attributes(), "outcome")
	require.True(t, ok)
	assert.Equal(t, OutcomeError, outcomeAttr.Value.AsString())

	var sawExceptionEvent bool
	for _, event := range span.Events() {
		if event.Name == "exception" {
			sawExceptionEvent = true
		}
	}
	assert.True(t, sawExceptionEvent, "expected RecordError to add an exception event")
}

// Story 1.3.1: the exception message and status description must carry no credential.
func TestWithOperationSpan_RedactsCredentialsInRecordedError(t *testing.T) {
	recorder := obstest.InstallProviders(t)

	leaky := errors.New("fatal: unable to access 'https://x-access-token:ghp_abc123@github.com/o/r.git/'")
	err := WithOperationSpan(context.Background(), "git.worktree.add", func() (string, string, error) {
		return "legacy", OutcomeError, leaky
	})
	require.ErrorIs(t, err, leaky, "the caller still gets the original error")

	span := recorder.Ended()[0]
	assert.NotContains(t, span.Status().Description, "ghp_")
	var messages []string
	for _, event := range span.Events() {
		for _, kv := range event.Attributes {
			messages = append(messages, kv.Value.String())
		}
	}
	require.NotEmpty(t, messages)
	for _, m := range messages {
		assert.NotContains(t, m, "ghp_")
	}
	assert.Contains(t, strings.Join(messages, "\n"), "https://***@github.com/o/r.git/")
}

// TestWithOperationSpan_AppliesCallerAttrsFromContext covers WithOperationAttrs: the
// session_name/worktree_path attribute a real dispatch point (Setup/removeLocked/Prune/
// findLiveWorktreeForBranch/MergeMainIntoWorktree) attaches via WithOperationAttrs must
// land on the span, even though WithOperationSpan's own signature has no room for it.
func TestWithOperationSpan_AppliesCallerAttrsFromContext(t *testing.T) {
	recorder := obstest.InstallProviders(t)

	ctx := WithOperationAttrs(context.Background(), attribute.String("session_name", "sess-1"))
	err := WithOperationSpan(ctx, "git.worktree.add", func() (string, string, error) {
		return "native", OutcomeSuccess, nil
	})
	require.NoError(t, err)

	ended := recorder.Ended()
	require.Len(t, ended, 1)

	sessionAttr, ok := obstest.FindSpanAttr(ended[0].Attributes(), "session_name")
	require.True(t, ok, "expected a session_name attribute carried over from WithOperationAttrs")
	assert.Equal(t, "sess-1", sessionAttr.Value.AsString())
}
