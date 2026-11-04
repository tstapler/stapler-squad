package main

import (
	"os"
	"strings"
	"testing"
)

func TestViolatedBoundaries(t *testing.T) {
	bs, err := ParseBoundaries(strings.NewReader("# c\nsession/prpoller !session/ent\nserver/services/devtools !server/web # why\n"))
	if err != nil || len(bs) != 2 {
		t.Fatalf("parse: %+v %v", bs, err)
	}
	if _, err := ParseBoundaries(strings.NewReader("onlyonefield\n")); err == nil {
		t.Fatal("a malformed line must fail")
	}
	deps := map[string][]string{"session/prpoller": {"session", "session/entity", "session/ent/ent"}, "server/services/devtools": {"config"}}
	got := ViolatedBoundaries(bs, deps)
	if len(got) != 1 || !strings.Contains(got[0], "session/prpoller must not depend on session/ent (reaches session/ent/ent)") {
		t.Fatalf("a sibling prefix (session/entity) must not match, a subpackage must: %v", got)
	}
}

// TestBoundariesSeededViolation is the Story 13.6 seeded-violation check against a real module.
func TestBoundariesSeededViolation(t *testing.T) {
	if os.Getenv("TEST_TMPDIR") != "" {
		t.Skip("needs the host go toolchain, unavailable in the Bazel sandbox")
	}
	root := t.TempDir()
	write(t, root, "go.mod", "module "+modulePath+"\n\ngo 1.21\n")
	write(t, root, "low/low.go", "package low\n\nfunc L() int { return 1 }\n")
	write(t, root, "mid/mid.go", "package mid\n\nimport \"github.com/tstapler/stapler-squad/low\"\n\nfunc M() int { return low.L() }\n")
	write(t, root, "top/top.go", "package top\n\nfunc T() int { return 1 }\n")
	write(t, root, "b.txt", "top !low\nmid !top\n")
	var out strings.Builder
	if code := runBoundaries([]string{"--root", root, "--file", "b.txt"}, &out); code != 0 {
		t.Fatalf("clean tree must pass: %d %s", code, out.String())
	}
	write(t, root, "top/top.go", "package top\n\nimport \"github.com/tstapler/stapler-squad/mid\"\n\nfunc T() int { return mid.M() }\n")
	out.Reset()
	if code := runBoundaries([]string{"--root", root, "--file", "b.txt"}, &out); code != 1 || !strings.Contains(out.String(), "top must not depend on low (reaches low)") {
		t.Fatalf("a transitive import of a forbidden package must fail: %d %s", code, out.String())
	}
}
