package backend

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"

	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/git/redact"
	"github.com/tstapler/stapler-squad/telemetry"
)

// LiveSessionProbe reports whether a running, non-paused session uses the repository root. The
// session manager supplies it; the Router calls it on every localwrite decision and never caches.
type LiveSessionProbe func(repoRoot string) bool

// Preflight is the capability preflight (ADR-006): the FallbackReasons that forbid serving this
// call in-process, or none. It runs before the non-CLI backend is touched, on every call routed
// to it, so a mutating call always sees fresh hook/signing state. Implementations map their own
// failures to ReasonCapabilityDetectError; a panic is treated the same way.
type Preflight func(ctx context.Context, loc Local, op OperationName) []FallbackReason

// RouterConfig is everything a Router needs. Zero values fail closed to the CLI.
type RouterConfig struct {
	Cohorts CohortMap
	// CLI serves Remote locations, every CLI-mode cohort and every fallback. Required.
	CLI Backend
	// GoGit is the in-process backend. Nil routes everything to the CLI (reason config).
	GoGit Backend
	// LocalWriteRepos is git_backend_localwrite_repos: the exact repo roots a localwrite call
	// may run in-process. Matching is on the cleaned path, so a subdirectory or an alias such
	// as a symlinked path is not a match; that errs toward the CLI.
	LocalWriteRepos []RepoRoot
	// LiveSessions guards localwrite. Nil means "assume a live session".
	LiveSessions LiveSessionProbe
	// Preflight is required for any call to reach GoGit; nil routes to the CLI with reason
	// capability_detect_error.
	Preflight Preflight
	// VerifiedWorktreeWriters lists operations that write worktree files and have a passing
	// fault-injection parity test. Any other such operation routes to the CLI
	// (unsafe_worktree_write).
	VerifiedWorktreeWriters map[OperationName]bool
	// Pinner captures HEAD and index state for shadow classification. Nil uses GoGit.
	Pinner StatePinner
	// Recorder receives real and false_clean shadow mismatches. Nil only logs them.
	Recorder MismatchRecorder
	// Meter registers the router's counters. Nil uses telemetry.GetMeter().
	Meter metric.Meter
}

// Router implements Backend by sending each call to the CLI or the in-process backend per the
// cohort map (ADR-004). Remote locations always go to the CLI; the CLI is also the fallback.
type Router struct {
	cohorts         CohortMap
	cli             Backend
	gogit           Backend
	localWriteRepos map[string]struct{}
	liveSessions    LiveSessionProbe
	preflight       Preflight
	worktreeWriters map[OperationName]bool
	pinner          StatePinner
	recorder        MismatchRecorder
	metrics         routerMetrics
}

var _ Backend = (*Router)(nil)

// NewRouter builds a Router. cfg.CLI is required.
func NewRouter(cfg RouterConfig) (*Router, error) {
	if cfg.CLI == nil {
		return nil, errors.New("git backend: router needs a CLI backend")
	}
	meter := cfg.Meter
	if meter == nil {
		meter = telemetry.GetMeter()
	}
	r := &Router{
		cohorts:         cfg.Cohorts,
		cli:             cfg.CLI,
		gogit:           cfg.GoGit,
		localWriteRepos: make(map[string]struct{}, len(cfg.LocalWriteRepos)),
		liveSessions:    cfg.LiveSessions,
		preflight:       cfg.Preflight,
		worktreeWriters: cfg.VerifiedWorktreeWriters,
		pinner:          cfg.Pinner,
		recorder:        cfg.Recorder,
		metrics:         newRouterMetrics(meter),
	}
	for _, root := range cfg.LocalWriteRepos {
		if root != "" {
			r.localWriteRepos[filepath.Clean(string(root))] = struct{}{}
		}
	}
	return r, nil
}

// operationCohort assigns every operation to the cohort that flips it (ADR-004).
var operationCohort = map[OperationName]Cohort{
	OpCurrentBranch: CohortRefs, OpHeadRef: CohortRefs, OpResolveRef: CohortRefs, OpRefExists: CohortRefs,
	OpRepoRoot: CohortRefs, OpGitDir: CohortRefs, OpCommonDir: CohortRefs, OpListRefs: CohortRefs,
	OpMergeBase: CohortRefs, OpCountCommits: CohortRefs, OpLog: CohortRefs, OpGetConfig: CohortRefs,
	OpListBranches: CohortRefs,

	OpIsDirty: CohortDiffStatus, OpStatus: CohortDiffStatus, OpListUntracked: CohortDiffStatus,
	OpDiff: CohortDiffStatus, OpDiffNumstat: CohortDiffStatus,

	OpSetConfig: CohortLocalWrite, OpSetRemoteURL: CohortLocalWrite, OpCreateBranch: CohortLocalWrite,
	OpRenameCurrentBranch: CohortLocalWrite, OpSwitchBranch: CohortLocalWrite, OpDiscardChanges: CohortLocalWrite,
	OpStashPush: CohortLocalWrite, OpStashPop: CohortLocalWrite, OpAdd: CohortLocalWrite,
	OpRestore: CohortLocalWrite, OpReset: CohortLocalWrite, OpDeleteBranch: CohortLocalWrite,
	OpSetUpstream: CohortLocalWrite, OpCheckoutCommit: CohortLocalWrite, OpRemoveFiles: CohortLocalWrite,
	OpMoveFile: CohortLocalWrite, OpCommit: CohortLocalWrite,

	OpListRemote: CohortNetwork, OpFetch: CohortNetwork, OpPull: CohortNetwork, OpPush: CohortNetwork,
	OpClone: CohortNetwork,

	OpListWorktrees: CohortWorktree, OpAddWorktree: CohortWorktree, OpAddWorktreeForExistingBranch: CohortWorktree,
	OpRemoveWorktree: CohortWorktree, OpPruneWorktrees: CohortWorktree,
}

// HasCohort reports whether op has an explicit cohort assignment (every operation must).
func HasCohort(op OperationName) bool { _, ok := operationCohort[op]; return ok }

// CohortOf returns the cohort an operation belongs to.
func CohortOf(op OperationName) Cohort { return operationCohort[op] }

// worktreeFileWriters are the operations that rewrite working-tree files (ADR-003 carve-out
// list: checkout, reset, restore, remove, move, plus the network and worktree operations that
// check files out).
var worktreeFileWriters = map[OperationName]bool{
	OpSwitchBranch: true, OpCheckoutCommit: true, OpDiscardChanges: true, OpStashPush: true,
	OpStashPop: true, OpRestore: true, OpReset: true, OpRemoveFiles: true, OpMoveFile: true,
	OpPull: true, OpClone: true, OpAddWorktree: true, OpAddWorktreeForExistingBranch: true,
	OpRemoveWorktree: true,
}

type routeKind uint8

const (
	routeCLI routeKind = iota
	routeGoGit
	routeShadow
	routeFail
)

// decision is where one call goes. count is true when the call was diverted from a non-CLI
// cohort mode, which is what git_backend_fallback_total counts.
type decision struct {
	kind   routeKind
	cohort Cohort
	reason FallbackReason
	count  bool
	local  Local
	err    error
}

func (r *Router) route(ctx context.Context, loc RepoLocation, op OperationName) decision {
	cohort := CohortOf(op)
	mode := r.cohorts.Mode(cohort)
	switch l := loc.(type) {
	case Remote:
		if isNilRunner(l.Runner) {
			return decision{kind: routeFail, cohort: cohort, err: ErrNoRemoteRunner}
		}
		return decision{kind: routeCLI, cohort: cohort, reason: ReasonRemoteHost, count: mode != BackendCLI}
	case Local:
		return r.routeLocal(ctx, l, op, cohort, mode)
	default:
		return decision{kind: routeFail, cohort: cohort, err: fmt.Errorf("%w: unsupported location %T", ErrInvalidArgument, loc)}
	}
}

func (r *Router) routeLocal(ctx context.Context, l Local, op OperationName, cohort Cohort, mode BackendMode) decision {
	toCLI := func(reason FallbackReason) decision {
		return decision{kind: routeCLI, cohort: cohort, reason: reason, count: true}
	}
	if mode == BackendCLI || (mode == BackendShadow && op.Mutating()) {
		return decision{kind: routeCLI, cohort: cohort, reason: ReasonConfig}
	}
	if r.gogit == nil {
		return toCLI(ReasonConfig)
	}
	if cohort == CohortLocalWrite {
		if _, ok := r.localWriteRepos[filepath.Clean(string(l.Root))]; !ok {
			return toCLI(ReasonConfig)
		}
	}
	reasons := r.preflightReasons(ctx, l, op)
	if cohort == CohortLocalWrite && (r.liveSessions == nil || r.liveSessions(string(l.Root))) {
		reasons = append(reasons, ReasonLiveSession)
	}
	if worktreeFileWriters[op] && !r.worktreeWriters[op] {
		reasons = append(reasons, ReasonUnsafeWorktreeWrite)
	}
	if reason := FirstReason(reasons...); reason != "" {
		return toCLI(reason)
	}
	kind := routeGoGit
	if mode == BackendShadow {
		kind = routeShadow
	}
	return decision{kind: kind, cohort: cohort, local: l}
}

func (r *Router) preflightReasons(ctx context.Context, l Local, op OperationName) (reasons []FallbackReason) {
	if r.preflight == nil {
		return []FallbackReason{ReasonCapabilityDetectError}
	}
	defer func() {
		if p := recover(); p != nil {
			log.Error("git capability preflight panicked, routing to cli", "operation", string(op), "panic", redact.Git(fmt.Sprint(p)))
			reasons = []FallbackReason{ReasonCapabilityDetectError}
		}
	}()
	return r.preflight(ctx, l, op)
}

func isNilRunner(r Runner) bool {
	if r == nil {
		return true
	}
	v := reflect.ValueOf(r)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

// readSpec describes how to judge a read's result "clean" for the destructive-intent rule and
// for shadow false_clean classification. clean is nil for operations with no such notion.
type readSpec[T any] struct {
	clean       func(T) bool
	destructive bool
}

func dispatch[T any](r *Router, ctx context.Context, loc RepoLocation, op OperationName, spec readSpec[T], call func(context.Context, Backend) (T, error)) (T, error) {
	d := r.route(ctx, loc, op)
	switch d.kind {
	case routeFail:
		var zero T
		return zero, d.err
	case routeGoGit:
		return goGitCall(r, ctx, op, d, spec, call)
	case routeShadow:
		return shadowCall(r, ctx, op, d, spec, call)
	default:
		if d.count {
			r.metrics.countFallback(op, d.cohort, d.reason)
		}
		return cliCall(r, ctx, op, call)
	}
}

func dispatchErr(r *Router, ctx context.Context, loc RepoLocation, op OperationName, call func(context.Context, Backend) error) error {
	_, err := dispatch(r, ctx, loc, op, readSpec[struct{}]{}, func(c context.Context, b Backend) (struct{}, error) {
		return struct{}{}, call(c, b)
	})
	return err
}

func cliCall[T any](r *Router, ctx context.Context, op OperationName, call func(context.Context, Backend) (T, error)) (T, error) {
	res, err := call(ctx, r.cli)
	r.noteError(ctx, op, ImplCLI, err)
	return res, err
}

// goGitCall runs the in-process backend and applies the fallback rules:
//   - a destructive-intent "clean" is confirmed by the CLI (never trusted);
//   - a failure falls back to the CLI only if nothing was written: for a mutating operation the
//     backend must say so with ErrFallbackEligible{Wrote:false} AND the call's CallState must be
//     unmarked, so a half-applied call is never replayed;
//   - every other failure is returned and counted in git_backend_error_total.
func goGitCall[T any](r *Router, ctx context.Context, op OperationName, d decision, spec readSpec[T], call func(context.Context, Backend) (T, error)) (T, error) {
	gctx, state := freshCallState(ctx)
	res, err := call(gctx, r.gogit)
	if err == nil {
		if spec.destructive && spec.clean != nil && spec.clean(res) {
			r.metrics.countFallback(op, d.cohort, ReasonDestructiveConfirm)
			return cliCall(r, ctx, op, call)
		}
		return res, nil
	}
	if reason, ok := fallbackReasonFor(ctx, op, err, state); ok {
		r.metrics.countFallback(op, d.cohort, reason)
		return cliCall(r, ctx, op, call)
	}
	err = withCallLevelWrote(err, state)
	r.noteError(ctx, op, ImplGoGit, err)
	return res, err
}

func freshCallState(ctx context.Context) (context.Context, *CallState) {
	s := &CallState{}
	return context.WithValue(ctx, callStateKey{}, s), s
}

// fallbackReasonFor decides whether a failed in-process call may be retried on the CLI.
func fallbackReasonFor(ctx context.Context, op OperationName, err error, state *CallState) (FallbackReason, bool) {
	if ctx.Err() != nil || state.Wrote() {
		return "", false
	}
	var eligible ErrFallbackEligible
	isEligible := errors.As(err, &eligible)
	if isEligible && eligible.Wrote {
		return "", false
	}
	if op.Mutating() && !isEligible {
		return "", false
	}
	switch {
	case errors.Is(err, ErrTornRead):
		return ReasonTornRead, true
	case errors.Is(err, ErrObjectMissing):
		return ReasonObjectMissing, true
	case errors.Is(err, ErrObjectNotFound):
		return ReasonObjectNotFound, true
	case isEligible:
		return ReasonError, true
	case isDomainError(err):
		return "", false
	}
	return ReasonError, true
}

// withCallLevelWrote makes a returned fallback-eligible error carry the call-level Wrote flag
// (the OR over every scope), so a caller that sees it never replays the operation.
func withCallLevelWrote(err error, state *CallState) error {
	var eligible ErrFallbackEligible
	if state.Wrote() && errors.As(err, &eligible) && !eligible.Wrote {
		return ErrFallbackEligible{Wrote: true, Err: eligible.Err}
	}
	return err
}

var domainErrors = []error{
	ErrUnborn, ErrDetachedHead, ErrRefNotFound, ErrNoMergeBase, ErrNotARepo, ErrConfigUnset,
	ErrNoRemoteRunner, ErrNoLocalRunner, ErrNothingToCommit, ErrInvalidArgument, ErrLocked{},
}

// isDomainError reports whether err is an expected answer ("no such ref") rather than a
// failure of the backend itself.
func isDomainError(err error) bool {
	for _, d := range domainErrors {
		if errors.Is(err, d) {
			return true
		}
	}
	return false
}

func (r *Router) noteError(ctx context.Context, op OperationName, impl Implementation, err error) {
	if err == nil || ctx.Err() != nil {
		return
	}
	if errors.Is(err, ErrFallbackEligible{}) || !isDomainError(err) {
		r.metrics.countError(op, impl)
	}
}
