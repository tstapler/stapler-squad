package backend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/git/redact"
)

// MismatchClass classifies a shadow-mode disagreement between the CLI and the in-process backend.
type MismatchClass string

const (
	// MismatchRacy: the difference disappeared on a pinned re-read, or the repository kept
	// changing underneath both reads. Reported, not gate-blocking.
	MismatchRacy MismatchClass = "racy"
	// MismatchReal: a stable disagreement. Blocks flipping the cohort.
	MismatchReal MismatchClass = "real"
	// MismatchFalseClean: a real mismatch where go-git said clean and the CLI said dirty.
	// Never waived.
	MismatchFalseClean MismatchClass = "false_clean"
)

// StatePin is the repository state a shadow comparison is pinned to. Valid is false when the
// state could not be read; an invalid pin never counts as "the repository moved".
type StatePin struct {
	Head       CommitSHA
	IndexMTime int64 // unix nanoseconds; 0 when there is no index file
	Valid      bool
}

// StatePinner captures the HEAD SHA and index file mtime of a repository.
type StatePinner func(ctx context.Context, loc Local) StatePin

// pinsMoved reports whether the repository demonstrably changed between two pins. Unreadable
// pins count as not moved so a mismatch is classified real (blocking) rather than hidden as racy.
func pinsMoved(a, b StatePin) bool { return a.Valid && b.Valid && a != b }

// MismatchRecord is the redacted repro record written for a real or false_clean mismatch. It
// holds no path or URL: the repository is identified by a hash of its root.
type MismatchRecord struct {
	Operation     OperationName `json:"operation"`
	Cohort        string        `json:"cohort"`
	Class         MismatchClass `json:"class"`
	CLIResult     string        `json:"cli_result"`
	GoGitResult   string        `json:"gogit_result"`
	HeadSHA       string        `json:"head_sha"`
	IndexMTimeNs  int64         `json:"index_mtime_ns"`
	RepoRootHash  string        `json:"repo_root_hash"`
	RecordedAtUTC string        `json:"recorded_at"`
}

// MismatchRecorder persists MismatchRecords for triage.
type MismatchRecorder interface {
	Record(MismatchRecord)
}

// FileMismatchRecorder writes each record as a JSON file in Dir (the config dir's
// shadow-mismatches/ directory in production).
type FileMismatchRecorder struct {
	Dir string
	seq atomic.Uint64
}

// Record implements MismatchRecorder. Failures are logged, never returned: recording must not
// affect the call that triggered it.
func (f *FileMismatchRecorder) Record(rec MismatchRecord) {
	if err := os.MkdirAll(f.Dir, 0o700); err != nil {
		log.Warn("cannot create shadow mismatch dir", "err", redact.Git(err.Error()))
		return
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		log.Warn("cannot encode shadow mismatch record", "err", err)
		return
	}
	name := fmt.Sprintf("%s-%06d-%s.json", time.Now().UTC().Format("20060102T150405Z"), f.seq.Add(1), rec.Operation)
	if err := os.WriteFile(filepath.Join(f.Dir, name), data, 0o600); err != nil {
		log.Warn("cannot write shadow mismatch record", "err", redact.Git(err.Error()))
	}
}

const maxRecordedResult = 2048

// outcome is one backend's answer to one read.
type outcome[T any] struct {
	res T
	err error
}

func (o outcome[T]) summary() string {
	var s string
	if o.err != nil {
		s = "error: " + o.err.Error()
	} else {
		s = fmt.Sprintf("%+v", o.res)
	}
	s = redact.Git(s)
	if len(s) > maxRecordedResult {
		s = s[:maxRecordedResult] + "...(truncated)"
	}
	return s
}

// shadowCall returns the CLI result and compares it with the in-process one. It never mutates
// (routeShadow is only produced for read operations) and the in-process side can neither change
// the returned value nor panic its way out.
func shadowCall[T any](r *Router, ctx context.Context, op OperationName, d decision, spec readSpec[T], call func(context.Context, Backend) (T, error)) (T, error) {
	r.metrics.countShadowCall(op)
	read := func() (outcome[T], outcome[T]) {
		c, cerr := call(ctx, r.cli)
		return outcome[T]{c, cerr}, safeGoGitRead(ctx, r.gogit, call)
	}

	pin0 := r.pin(ctx, d.local)
	cliFirst, goFirst := read()
	if ctx.Err() != nil || outcomesEqual(cliFirst, goFirst) {
		r.noteError(ctx, op, ImplCLI, cliFirst.err)
		return cliFirst.res, cliFirst.err
	}

	// Disagreement: re-read both once, bracketed by pins taken before the first read and after
	// the re-read. Only a pair read while HEAD and the index stood still counts.
	cliSecond, goSecond := read()
	stableCLI, stableGo, stable := cliSecond, goSecond, !pinsMoved(pin0, r.pin(ctx, d.local))
	if !stable {
		pinA := r.pin(ctx, d.local)
		stableCLI, stableGo = read()
		stable = !pinsMoved(pinA, r.pin(ctx, d.local))
		pin0 = pinA
	}

	switch {
	case !stable, outcomesEqual(stableCLI, stableGo):
		r.metrics.countMismatch(op, MismatchRacy)
	default:
		class := MismatchReal
		if spec.clean != nil && stableCLI.err == nil && stableGo.err == nil &&
			spec.clean(stableGo.res) && !spec.clean(stableCLI.res) {
			class = MismatchFalseClean
		}
		r.metrics.countMismatch(op, class)
		r.reportMismatch(op, d, class, pin0, stableCLI.summary(), stableGo.summary())
	}
	r.noteError(ctx, op, ImplCLI, cliFirst.err)
	return cliFirst.res, cliFirst.err
}

func safeGoGitRead[T any](ctx context.Context, b Backend, call func(context.Context, Backend) (T, error)) (res outcome[T]) {
	defer func() {
		if p := recover(); p != nil {
			res = outcome[T]{err: fmt.Errorf("git backend: in-process backend panicked: %v", p)}
		}
	}()
	gctx, _ := freshCallState(ctx)
	v, err := call(gctx, b)
	return outcome[T]{v, err}
}

func (r *Router) reportMismatch(op OperationName, d decision, class MismatchClass, pin StatePin, cliSummary, goSummary string) {
	rec := MismatchRecord{
		Operation:     op,
		Cohort:        d.cohort.String(),
		Class:         class,
		CLIResult:     cliSummary,
		GoGitResult:   goSummary,
		HeadSHA:       string(pin.Head),
		IndexMTimeNs:  pin.IndexMTime,
		RepoRootHash:  rootHash(d.local.Root),
		RecordedAtUTC: time.Now().UTC().Format(time.RFC3339),
	}
	log.Warn("git backend shadow mismatch",
		"op", string(op), "cohort", rec.Cohort, "class", string(class), "repo", rec.RepoRootHash,
		"cli", cliSummary, "gogit", goSummary)
	if r.recorder != nil {
		r.recorder.Record(rec)
	}
}

func rootHash(root RepoRoot) string {
	sum := sha256.Sum256([]byte(root))
	return hex.EncodeToString(sum[:8])
}

// pin captures the current state, defaulting to the in-process backend's view.
func (r *Router) pin(ctx context.Context, loc Local) StatePin {
	if r.pinner != nil {
		return r.pinner(ctx, loc)
	}
	return defaultPin(ctx, r.gogit, loc)
}

func defaultPin(ctx context.Context, b Backend, loc Local) (pin StatePin) {
	defer func() {
		if recover() != nil {
			pin = StatePin{}
		}
	}()
	head, err := b.ResolveRef(ctx, loc, "HEAD")
	if err != nil && !errors.Is(err, ErrUnborn) {
		return StatePin{}
	}
	dir, err := b.GitDir(ctx, loc)
	if err != nil {
		return StatePin{}
	}
	gitDir := string(dir)
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(loc.Dir(), gitDir)
	}
	pin = StatePin{Head: head, Valid: true}
	switch info, err := os.Stat(filepath.Join(gitDir, "index")); {
	case err == nil:
		pin.IndexMTime = info.ModTime().UnixNano()
	case !errors.Is(err, os.ErrNotExist):
		return StatePin{}
	}
	return pin
}

// outcomesEqual compares two answers. Errors agree when both are nil, share a sentinel
// (ErrUnborn, ErrRefNotFound, ...) or both are undifferentiated failures.
func outcomesEqual[T any](a, b outcome[T]) bool {
	if (a.err == nil) != (b.err == nil) {
		return false
	}
	if a.err != nil {
		return sameErrorClass(a.err, b.err)
	}
	return deepEqual(reflect.ValueOf(a.res), reflect.ValueOf(b.res))
}

func sameErrorClass(a, b error) bool {
	for _, d := range domainErrors {
		if errors.Is(a, d) != errors.Is(b, d) {
			return false
		}
	}
	return true
}

// deepEqual is reflect.DeepEqual except that a nil and an empty slice or map are equal, since
// two backends legitimately differ on which one "nothing" is.
func deepEqual(a, b reflect.Value) bool {
	if !a.IsValid() || !b.IsValid() {
		return a.IsValid() == b.IsValid()
	}
	if a.Type() != b.Type() {
		return false
	}
	switch a.Kind() {
	case reflect.Slice:
		if a.Len() != b.Len() {
			return false
		}
		for i := 0; i < a.Len(); i++ {
			if !deepEqual(a.Index(i), b.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Array:
		for i := 0; i < a.Len(); i++ {
			if !deepEqual(a.Index(i), b.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Map:
		if a.Len() != b.Len() {
			return false
		}
		for _, k := range a.MapKeys() {
			bv := b.MapIndex(k)
			if !bv.IsValid() || !deepEqual(a.MapIndex(k), bv) {
				return false
			}
		}
		return true
	case reflect.Struct:
		for i := 0; i < a.NumField(); i++ {
			if !deepEqual(a.Field(i), b.Field(i)) {
				return false
			}
		}
		return true
	case reflect.Ptr, reflect.Interface:
		if a.IsNil() || b.IsNil() {
			return a.IsNil() == b.IsNil()
		}
		return deepEqual(a.Elem(), b.Elem())
	case reflect.String:
		return a.String() == b.String()
	case reflect.Bool:
		return a.Bool() == b.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return a.Int() == b.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return a.Uint() == b.Uint()
	case reflect.Float32, reflect.Float64:
		return a.Float() == b.Float()
	default:
		return false
	}
}
