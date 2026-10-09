package backend_test

import (
	"reflect"
	"sort"
	"testing"

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
	var ops []string
	for _, op := range backend.AllOperations() {
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
