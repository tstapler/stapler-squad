package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHandWrittenListMatch(t *testing.T) {
	dir := t.TempDir()
	list := filepath.Join(dir, "list.txt")
	os.WriteFile(list, []byte("# comment\nproto/session/v1/BUILD.bazel  # generated seam\nBUILD.bazel\n"), 0o644)
	for path, want := range map[string]bool{"proto/session/v1/BUILD.bazel": true, "./BUILD.bazel": true, "session/BUILD.bazel": false} {
		got, err := handWritten(list, path)
		if err != nil || got != want {
			t.Errorf("%s: got %v %v", path, got, err)
		}
	}
	if _, err := handWritten(filepath.Join(dir, "missing"), "x"); err == nil {
		t.Error("an unreadable list must be an error so the driver refuses")
	}
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	c.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestDriverKeepsOursMarksPendingAndFinishIsNoopWithoutMarker(t *testing.T) {
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	old, _ := os.Getwd()
	defer os.Chdir(old)
	os.Chdir(dir)
	os.MkdirAll("tools/bzl", 0o755)
	os.WriteFile("tools/bzl/hand-build-files.txt", []byte("hand/BUILD.bazel\n"), 0o644)
	if code := run([]string{"base", "ours", "theirs", "hand/BUILD.bazel"}); code != 1 {
		t.Fatalf("hand-written file must be refused, got %d", code)
	}
	if code := finish(); code != 0 {
		t.Fatalf("finish without marker: %d", code)
	}
	if code := run([]string{"base", "ours", "theirs", "gen/BUILD.bazel"}); code != 0 {
		t.Fatalf("generated file must be accepted, got %d", code)
	}
	if _, err := os.Stat(filepath.Join(".git", pendingMarker)); err != nil {
		t.Fatalf("marker missing: %v", err)
	}
}
