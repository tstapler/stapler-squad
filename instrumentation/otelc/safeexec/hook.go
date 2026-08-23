//go:build otelcauto

// Package safeexec is an otelc compile-time instrumentation hook package
// (see otelc.yaml in this directory), not the executor/safeexec package it
// instruments. It wraps executor/safeexec.CommandContext — the module's
// single subprocess-spawning choke point — with a span per invocation.
//
// Build-tagged (otelcauto) because it imports go.opentelemetry.io/otelc/pkg/hook,
// which is only ever present in go.mod via the `replace` directive `otelc
// setup` adds and `otelc cleanup` removes (scripts/otel-auto-build.sh) — a
// plain `go build ./...`/`make build`/`make lint` never runs `otelc setup`,
// so that import is genuinely unsatisfied outside the otelc-auto build. The
// tag keeps this package out of the default build's package graph entirely
// (confirmed empirically, 2026-08-22: without it, `go build ./...` fails
// with "no required module provides package .../pkg/hook" even though
// nothing in the default build imports this package — `./...` still tries
// to compile every directory under the module root). scripts/otel-auto-build.sh
// injects `-tags otelcauto` (merged with any caller-supplied `-tags`, e.g.
// `embed_tmux`) into both its `otelc setup` calls and the actual build/test
// invocation, so the tag is active exactly when — and only when — this
// package needs to compile.
//
// CommandContext returns a single *exec.Cmd with no error: it only
// constructs the command, it never runs it. So "before"/"after" here bracket
// Cmd construction, not the subprocess's actual execution — that happens
// later in the caller's own cmd.Run()/cmd.Wait()/cmd.Output(), outside this
// hook's reach. The only failure observable at this hook site is whether the
// caller's ctx was already canceled or past its deadline by the time
// CommandContext returned; that is what gets recorded as a span error below.
// Genuine subprocess exit-code failures are not visible here — see
// project_plans/go-auto-instrumentation/implementation/spike-verdicts.md
// (Spike E) for this known limitation and the reasoning behind it.
//
// The before/after signature (first parameter hook.HookContext, "before"
// mirrors the target function's own parameters, "after" mirrors its return
// values) was reverse-engineered from otelc's fetched built-in rule sources
// (.otelc-build/instrumentation/{net/http/client,database/sql,...}) during a
// real `otelc setup` run — it is not spelled out in otelc's docs for a
// single-return, no-error function shape like CommandContext's. See Spike E
// for the full trail.
package safeexec

import (
	"context"
	"os/exec"
	"sync/atomic"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"go.opentelemetry.io/otelc/pkg/hook"

	"github.com/tstapler/stapler-squad/telemetry"
)

const instrumentationName = "github.com/tstapler/stapler-squad/instrumentation/otelc/safeexec"

// tracer is a package-level var (not injected via a constructor) because
// otelc invokes BeforeCommandContext/AfterCommandContext as bare
// package-level functions with a fixed signature — there is no call site
// that constructs this package, so no place to pass a tracer through.
var tracer = otel.Tracer(instrumentationName)

// callState carries the span created by BeforeCommandContext through to
// AfterCommandContext via hook.HookContext's Set/GetData, and guards against
// AfterCommandContext ending the span more than once. ctx is stored here
// against the usual Go guidance to not hold a Context in a struct — it is
// stored only because hook.HookContext's SetData/GetData is the sole bridge
// otelc provides between the Before and After hook calls, with no other way
// to carry it across.
type callState struct {
	span  trace.Span
	ctx   context.Context
	ended atomic.Bool
}

// BeforeCommandContext starts a span for a safeexec.CommandContext call,
// named for the subprocess being launched, carrying the typed
// telemetry.AttrSubprocessCommand / telemetry.AttrSubprocessArgCount
// attributes (telemetry/attributes.go), and stashes it for
// AfterCommandContext to close out.
func BeforeCommandContext(ictx hook.HookContext, ctx context.Context, name string, arg ...string) {
	_, span := tracer.Start(ctx, name, trace.WithAttributes(
		telemetry.SubprocessCommandAttr(name),
		telemetry.SubprocessArgCountAttr(len(arg)),
	))
	ictx.SetData(&callState{span: span, ctx: ctx})
}

// AfterCommandContext ends the span started by BeforeCommandContext. cmd
// (exec.CommandContext's own return value) is unused — see the package doc
// for why a failed *subprocess run* is not observable here.
func AfterCommandContext(ictx hook.HookContext, _ *exec.Cmd) {
	state, ok := ictx.GetData().(*callState)
	if !ok || state == nil || state.span == nil {
		return // before-hook didn't run (e.g. instrumentation disabled) — nothing to close
	}
	if !state.ended.CompareAndSwap(false, true) {
		return // already ended — guards against a double span.End()
	}
	defer state.span.End()

	if state.ctx != nil {
		if err := state.ctx.Err(); err != nil {
			state.span.RecordError(err)
			state.span.SetStatus(codes.Error, err.Error())
		}
	}
}
