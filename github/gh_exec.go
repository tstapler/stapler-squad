package github

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/tstapler/stapler-squad/telemetry"
)

// runGHCLICommand is the shared instrumentation core for every `gh` CLI
// subprocess invocation — both github/*.go's safeexec.CommandContext sites
// and session/git/worktree_git.go's tmux.CommandRunner-routed sites funnel
// through here (directly or via GitWorktree.runGHCommand) — so both adapters
// emit identical span/metric telemetry from one place. It records the
// instrumentation, then returns exec()'s result completely unchanged: no
// error wrapping, so callers' existing error-handling (e.g. *exec.ExitError
// type assertions on the underlying error) keeps working exactly as before.
//
// callSite names the actual gh subcommand invoked (e.g. "pr.view",
// "pr.merge") and becomes both the span name suffix and the
// github.call_site attribute/metric label — see plan.md's Observability
// Plan cardinality rule (call_site is a fixed, bounded enum, never a
// formatted string).
func runGHCLICommand(ctx context.Context, callSite string, exec func() ([]byte, error)) ([]byte, error) {
	ctx, span := telemetry.StartSpan(ctx, "gh."+callSite, trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	origin := GitHubCallOriginFrom(ctx)
	attrs := []attribute.KeyValue{
		attribute.String("process.command", "gh"),
		attribute.String("github.call.origin", string(origin)),
		attribute.String("github.call_site", callSite),
	}
	span.SetAttributes(attrs...)

	start := time.Now()
	output, err := exec()
	duration := time.Since(start)

	recordGitHubCall(ctx, duration, attrs)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return output, err
}
