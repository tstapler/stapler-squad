package svctest

import (
	"reflect"
	"testing"
)

// assertConfigChangesBehavior is the shared proof helper: it runs observe with
// each setting value and fails unless the observable differs. Every proof test
// named in the registry must call it.
func AssertConfigChangesBehavior(t testing.TB, observe func(setting any) any, before, after any) {
	t.Helper()
	b, a := observe(before), observe(after)
	if reflect.DeepEqual(b, a) {
		t.Fatalf("setting %v -> %v did not change the observable (%v): config has no runtime effect", before, after, b)
	}
}
