package main

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

func TestNegate(t *testing.T) {
	cases := []struct{ kind, in, want string }{
		{"go", "\tif x > 3 {", "\tif !(x > 3) {"},
		{"go", "\tif err := f(); err != nil {", ""},
		{"go", "\t} else if y {", ""},
		{"web", "  if (a && b) {", "  if (!(a && b)) {"},
		{"web", "  if (a) return;", ""},
	}
	for _, c := range cases {
		if got := negate(c.kind, c.in); got != c.want {
			t.Errorf("%s %q: got %q want %q", c.kind, c.in, got, c.want)
		}
	}
}

func TestApplyAndRevert(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "x.go")
	orig := "a\n\tif ok {\nb\n"
	_ = os.WriteFile(p, []byte(orig), 0o644)
	m := Mutation{File: "x.go", Line: 2, Before: "\tif ok {", After: "\tif !(ok) {"}
	if err := apply(d, m, true); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "a\n\tif !(ok) {\nb\n" {
		t.Fatalf("%q", b)
	}
	if err := apply(d, m, true); err == nil {
		t.Fatal("double apply must fail")
	}
	if err := apply(d, m, false); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != orig {
		t.Fatalf("not reverted: %q", b)
	}
}

func TestPickDistinctKeysAndRange(t *testing.T) {
	c := map[string][]Mutation{
		"a/x.go": {{File: "a/x.go", Package: "a"}}, "a/y.go": {{File: "a/y.go", Package: "a"}},
		"b/z.go": {{File: "b/z.go", Package: "b"}}, "c/w.go": {{File: "c/w.go", Package: "c"}},
	}
	specs := map[string]int{"a/x.go": 5, "a/y.go": 5, "b/z.go": 5, "c/w.go": 500}
	got := pick(c, specs, func(m Mutation) string { return m.Package }, 10, 2, 60, rand.New(rand.NewSource(1)), map[string]bool{})
	if len(got) != 2 {
		t.Fatalf("want one per package in range (a,b): %v", got)
	}
}
