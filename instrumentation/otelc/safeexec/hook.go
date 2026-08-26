//go:build otelcauto

// Package safeexec is an otelc compile-time instrumentation hook package
// (see otelc.yaml in this directory), not the executor/safeexec package it
// instruments — it wraps executor/safeexec.CommandContext, the module's
// single subprocess-spawning choke point, with a span per invocation.
// Build-tagged otelcauto because it imports go.opentelemetry.io/otelc/pkg/hook,
// which exists in go.mod only inside scripts/otel-auto-build.sh's `otelc
// setup` window.
//
// CommandContext only constructs the *exec.Cmd, never runs it — before/after
// here bracket construction, not execution, so a genuine subprocess exit-code
// failure is never observable at this hook site. See
// project_plans/go-auto-instrumentation/implementation/spike-verdicts.md
// (Spike E) for why, and for the before/after signature convention.
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
