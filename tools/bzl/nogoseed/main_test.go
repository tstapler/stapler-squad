package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

const captured = `ERROR: nogo failed
tools/bzl/nogo/testdata/seed/norawexec/seed.go:7:43: direct call to os/exec.Command (norawexec)
tools/bzl/nogo/testdata/seed/norawgitcli/seed.go:7:43: exec.Command invokes the git CLI directly (norawgitcli)
tools/bzl/nogo/testdata/seed/norawgitcli/seed.go:7:43: direct call to os/exec.Command (norawexec)
some/real/file.go:1:1: a finding outside the seeds (hotpolllog)
`

func TestMissingFindings(t *testing.T) {
	got := missingFindings([]string{"norawexec", "norawgitcli", "hotpolllog", "norawdb"}, captured)
	want := []string{"hotpolllog", "norawdb"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestNameIsNotAPrefixMatch(t *testing.T) {
	out := "tools/bzl/nogo/testdata/seed/norawgitopen/seed.go:3:1: x (norawgitopen)\n"
	if got := missingFindings([]string{"norawgit"}, out); len(got) != 1 {
		t.Fatalf("got %v", got)
	}
}

func TestWithFullLogAppendsTheWrapperLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "last-build.log")
	if err := os.WriteFile(path, []byte("tools/bzl/nogo/testdata/seed/norawdb/seed.go:3:1: x (norawdb)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := withFullLog([]byte("... 15 more ERROR lines not shown\nfull log: " + path + "\n"))
	if got := missingFindings([]string{"norawdb"}, string(out)); len(got) != 0 {
		t.Fatalf("finding in the full log not seen: %v", got)
	}
}

func TestWithFullLogWithoutPointerIsUnchanged(t *testing.T) {
	if got := withFullLog([]byte("plain")); string(got) != "plain" {
		t.Fatalf("got %q", got)
	}
}
