package gitwiring

import (
	"context"
	"testing"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

type fakeLocal struct{ calls [][]string }

func (f *fakeLocal) Run(_ context.Context, _, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	return []byte("main\n"), nil
}

func TestNewRouterRoutesEveryCohortToCLIWithoutGoGit(t *testing.T) {
	var cohorts backend.CohortMap
	for _, c := range backend.AllCohorts() {
		cohorts = cohorts.With(c, backend.BackendGoGit)
	}
	local := &fakeLocal{}
	b := NewRouter(cohorts, local)
	got, err := b.CurrentBranch(context.Background(), backend.Local{Root: "/r"})
	if err != nil || got != "main" || len(local.calls) != 1 {
		t.Fatalf("CurrentBranch = %q, %v, calls=%v", got, err, local.calls)
	}
}
