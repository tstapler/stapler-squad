package github

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// withTestTracerProvider installs an in-memory span recorder as the global
// TracerProvider for the duration of a test and returns it, restoring the
// previous provider on cleanup. telemetry.GetTracer() falls through to
// otel.Tracer(...) whenever telemetry.Initialize was never called (true for
// this test binary), so this is sufficient to observe spans runGHCLICommand
// emits without depending on the full telemetry.Provider setup.
func withTestTracerProvider(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	prev := otel.GetTracerProvider()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prev)
	})
	return sr
}

func TestRunGHCLICommand_should_EmitSpanWithOriginAndCallSite_When_ExecSucceeds(t *testing.T) {
	sr := withTestTracerProvider(t)

	ctx := WithGitHubCallOrigin(context.Background(), OriginPRStatusPoller)
	wantOutput := []byte("some gh output")

	output, err := runGHCLICommand(ctx, "pr.view", func() ([]byte, error) {
		return wantOutput, nil
	})
	if err != nil {
		t.Fatalf("runGHCLICommand returned unexpected error: %v", err)
	}
	if string(output) != string(wantOutput) {
		t.Fatalf("runGHCLICommand output = %q, want %q", output, wantOutput)
	}

	ended := sr.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(ended))
	}
	span := ended[0]
	if span.Name() != "gh.pr.view" {
		t.Errorf("span name = %q, want %q", span.Name(), "gh.pr.view")
	}

	attrs := map[string]string{}
	for _, a := range span.Attributes() {
		attrs[string(a.Key)] = a.Value.AsString()
	}
	if got := attrs["process.command"]; got != "gh" {
		t.Errorf("process.command = %q, want %q", got, "gh")
	}
	if got := attrs["github.call.origin"]; got != string(OriginPRStatusPoller) {
		t.Errorf("github.call.origin = %q, want %q", got, OriginPRStatusPoller)
	}
	if got := attrs["github.call_site"]; got != "pr.view" {
		t.Errorf("github.call_site = %q, want %q", got, "pr.view")
	}
}

func TestRunGHCLICommand_should_PassThroughErrorUnwrapped_When_ExecFails(t *testing.T) {
	withTestTracerProvider(t)

	wantErr := errors.New("boom")
	ctx := WithGitHubCallOrigin(context.Background(), OriginInteractive)

	output, err := runGHCLICommand(ctx, "pr.merge", func() ([]byte, error) {
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("runGHCLICommand error = %v, want exactly %v (unwrapped)", err, wantErr)
	}
	if output != nil {
		t.Fatalf("runGHCLICommand output = %v, want nil", output)
	}
}
