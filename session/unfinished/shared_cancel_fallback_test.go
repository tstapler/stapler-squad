package unfinished

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
)

// cancelAtFallbackCheck is live everywhere except at the fallback decision: only
// a ctx.Err() call made by the named functions reports Canceled. That models a
// cancellation landing between the failed graph walk and the legacy rerun
// without any timing (the walk itself sees a live ctx and returns ErrFallback).
type cancelAtFallbackCheck struct{ context.Context }

func (cancelAtFallbackCheck) Err() error {
	pc, _, _, ok := runtime.Caller(1)
	if !ok {
		return nil
	}
	name := runtime.FuncForPC(pc).Name()
	for _, f := range []string{"aheadBehindShared", "sharedHandled", "sharedErr"} {
		if strings.Contains(name, f) {
			return context.Canceled
		}
	}
	return nil
}

func tinyGraphReader(t *testing.T) (r *GoGitVCSReader, dir string) {
	t.Helper()
	_, dir, _ = testrepo.Diverged(t, 40, 3, 2)
	r = readerWithFlags(Flags{SharedCommitState: true}, &GoGitVCSReader{repoStateMaxGraphCommits: 8})
	return r, dir
}

func TestAheadBehind_ShouldReturnCtxError_WhenFallbackRacesCancellation(t *testing.T) {
	r, dir := tinyGraphReader(t)
	_, _, err := r.AheadBehind(cancelAtFallbackCheck{context.Background()}, dir, "base")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled, not a rerun of the legacy walk", err)
	}
}

func TestCommitMessages_ShouldReturnCtxError_WhenFallbackRacesCancellation(t *testing.T) {
	r, dir := tinyGraphReader(t)
	_, err := r.CommitMessages(cancelAtFallbackCheck{context.Background()}, dir, "base", 5)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled, not a rerun of the legacy walk", err)
	}
}
