package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T, onDisk, inBuild, inLinter []string) string {
	root := t.TempDir()
	for _, n := range onDisk {
		write(t, root, "tools/lint/"+n+"/analyzer.go", "package "+n)
	}
	build := "x = 1\nANALYZERS = [\n"
	for _, n := range inBuild {
		build += "    \"" + n + "\",\n"
	}
	write(t, root, "tools/bzl/nogo/BUILD.bazel", build+"]\n")
	main := "import (\n"
	for _, n := range inLinter {
		main += "\t\"github.com/tstapler/stapler-squad/tools/lint/" + n + "\"\n"
	}
	write(t, root, "tools/lint/cmd/linter/main.go", main+")\n")
	return root
}

func TestAgreeing(t *testing.T) {
	two := []string{"norawexec", "hotpolllog"}
	problems, err := check(fixture(t, two, two, two))
	if err != nil || len(problems) != 0 {
		t.Fatalf("problems=%v err=%v", problems, err)
	}
}

func TestMissingTwinAndMissingImport(t *testing.T) {
	problems, err := check(fixture(t, []string{"a", "b"}, []string{"a"}, []string{"b"}))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(problems, "\n")
	for _, want := range []string{
		`"b" is in tools/lint/*/analyzer.go but missing from tools/bzl/nogo ANALYZERS`,
		`"a" is in tools/lint/*/analyzer.go but missing from cmd/linter imports`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestStaleListEntry(t *testing.T) {
	problems, _ := check(fixture(t, []string{"a"}, []string{"a", "gone"}, []string{"a"}))
	if len(problems) != 1 || !strings.Contains(problems[0], `"gone" is in tools/bzl/nogo ANALYZERS`) {
		t.Fatalf("problems=%v", problems)
	}
}

func TestNoList(t *testing.T) {
	root := fixture(t, nil, nil, nil)
	write(t, root, "tools/bzl/nogo/BUILD.bazel", "nothing")
	if _, err := check(root); err == nil {
		t.Fatal("want error")
	}
}
