package backend_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

func backendMethods() reflect.Type { return reflect.TypeOf((*backend.Backend)(nil)).Elem() }

func TestOperationsMatchBackendMethods(t *testing.T) {
	typ := backendMethods()
	var methods []string
	for i := 0; i < typ.NumMethod(); i++ {
		methods = append(methods, typ.Method(i).Name)
	}
	all := backend.AllOperations()
	ops := make([]string, 0, len(all))
	for _, op := range all {
		assert.True(t, op.Known())
		ops = append(ops, string(op))
	}
	sort.Strings(methods)
	sort.Strings(ops)
	assert.Equal(t, methods, ops, "OperationName set must equal the Backend method set")
}

func TestEveryMethodTakesLocationFirst(t *testing.T) {
	typ := backendMethods()
	loc := reflect.TypeOf((*backend.RepoLocation)(nil)).Elem()
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		require.GreaterOrEqual(t, m.Type.NumIn(), 2, m.Name)
		assert.Equal(t, loc, m.Type.In(1), "%s: RepoLocation must follow ctx", m.Name)
	}
}

// Adjacent parameters of the same string kind are the swap hazard the primitive-obsession
// checklist exists for; wrap them in a request struct instead.
func TestNoAdjacentSameTypedParams(t *testing.T) {
	typ := backendMethods()
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		for p := 2; p < m.Type.NumIn(); p++ {
			a, b := m.Type.In(p-1), m.Type.In(p)
			if a.Kind() == reflect.String && a == b {
				assert.Failf(t, "adjacent same-typed string parameters", "%s: %v then %v", m.Name, a, b)
			}
		}
	}
}

func TestMutatingFlag(t *testing.T) {
	for _, op := range []backend.OperationName{backend.OpCommit, backend.OpPush, backend.OpAdd, backend.OpRemoveWorktree, backend.OpClone} {
		assert.True(t, op.Mutating(), op)
	}
	for _, op := range []backend.OperationName{backend.OpCurrentBranch, backend.OpStatus, backend.OpDiff, backend.OpListWorktrees, backend.OpLog} {
		assert.False(t, op.Mutating(), op)
	}
	assert.False(t, backend.OperationName("Bogus").Known())
}

func TestIntentZeroValueIsDestructive(t *testing.T) {
	var zero backend.Intent
	assert.Equal(t, backend.IntentDestructive, zero)
	assert.Equal(t, "display", backend.IntentDisplay.String())
}

func TestLocationDir(t *testing.T) {
	assert.Equal(t, "/r", backend.Local{Root: "/r"}.Dir())
	assert.Equal(t, "/p", backend.Remote{Host: "h", Path: "/p"}.Dir())
}

func TestRequiredGitEnv(t *testing.T) {
	env := backend.RequiredGitEnv()
	assert.Contains(t, env, "LC_ALL=C")
	assert.Contains(t, env, "GIT_TERMINAL_PROMPT=0")
	env[0] = "mutated"
	assert.NotEqual(t, "mutated", backend.RequiredGitEnv()[0], "a fresh slice per call")
}

func TestErrLockedCarriesDetailAndMatchesByType(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", backend.ErrLocked{Path: "/r/.git/index.lock", Age: time.Minute, Journaled: true})
	assert.ErrorIs(t, err, backend.ErrLocked{})
	var l backend.ErrLocked
	require.ErrorAs(t, err, &l)
	assert.Equal(t, "/r/.git/index.lock", l.Path)
	assert.Equal(t, time.Minute, l.Age)
	assert.True(t, l.Journaled)
	assert.NotErrorIs(t, errors.New("other"), backend.ErrLocked{})
}

func TestErrFallbackEligibleShape(t *testing.T) {
	cause := errors.New("object missing")
	err := fmt.Errorf("op: %w", backend.ErrFallbackEligible{Wrote: true, Err: cause})
	assert.ErrorIs(t, err, backend.ErrFallbackEligible{})
	assert.ErrorIs(t, err, cause)
	var fe backend.ErrFallbackEligible
	require.ErrorAs(t, err, &fe)
	assert.True(t, fe.Wrote)
}

func TestCallStateIsCallLevel(t *testing.T) {
	assert.Nil(t, backend.CallStateFrom(context.Background()))
	ctx, st := backend.WithCallState(context.Background())
	assert.False(t, st.Wrote())
	backend.CallStateFrom(ctx).MarkWrote() // a nested scope marks the shared call state
	assert.True(t, st.Wrote())
}
