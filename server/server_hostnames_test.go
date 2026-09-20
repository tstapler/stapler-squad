package server

import (
	"reflect"
	"sync"
	"testing"
)

// setHostnamesMergeCases are the table cases for
// TestServer_SetHostnames_AddOnlyMerge, pulled out to a package-level var so
// the test function itself stays short.
var setHostnamesMergeCases = []struct {
	name  string
	calls [][]string
	want  []string
}{
	{
		name:  "single call publishes the given set",
		calls: [][]string{{"netflix1.staplerhome.com"}},
		want:  []string{"netflix1.staplerhome.com"},
	},
	{
		name: "new network adds without dropping the old one",
		calls: [][]string{
			{"netflix1.staplerhome.com"},
			{"netflix1.staplerhome.com", "netflix1.newwifi.local"},
		},
		want: []string{"netflix1.staplerhome.com", "netflix1.newwifi.local"},
	},
	{
		name: "duplicate entries within a single call are deduped",
		calls: [][]string{
			{"a.local", "a.local", "b.local"},
		},
		want: []string{"a.local", "b.local"},
	},
	{
		name: "third call still preserves prior order and appends new entries",
		calls: [][]string{
			{"a.local"},
			{"a.local", "b.local"},
			{"c.local", "a.local"},
		},
		want: []string{"a.local", "b.local", "c.local"},
	},
}

// TestServer_SetHostnames_AddOnlyMerge verifies SetHostnames computes the
// add-only union of the previously published slice and each new candidate
// slice, deduping and never shrinking the set (Task 1.1.1b).
func TestServer_SetHostnames_AddOnlyMerge(t *testing.T) {
	for _, tt := range setHostnamesMergeCases {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{}
			for _, call := range tt.calls {
				s.SetHostnames(call)
			}

			got := s.GetHostnames()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetHostnames() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestServer_SetHostnames_ConcurrentAccessIsRaceFree spawns a writer and a
// reader goroutine for a bounded number of iterations and joins both before
// returning, to confirm GetHostnames never data-races with a concurrent
// SetHostnames (Task 1.1.1b; run with -race).
func TestServer_SetHostnames_ConcurrentAccessIsRaceFree(t *testing.T) {
	const iterations = 1000

	s := &Server{}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			s.SetHostnames([]string{"a.local", "b.local"})
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = s.GetHostnames()
		}
	}()

	wg.Wait()

	got := s.GetHostnames()
	want := []string{"a.local", "b.local"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetHostnames() after concurrent access = %v, want %v", got, want)
	}
}
