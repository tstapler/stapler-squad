package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeCodesign prints a requirement chosen by the last argument's base name.
func fakeCodesign(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "codesign")
	body := "#!/bin/sh\ncase \"$3\" in *odd) echo 'designated => cdhash H\"1\"';; *) echo 'designated => anchor H\"abc\"';; esac\n"
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func runArgs(args ...string) (int, string) {
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
	return code, out.String() + errb.String()
}

func TestIdentical(t *testing.T) {
	code, out := runArgs("-make-bin", "/a/one", "-bazel-bin", "/b/two", "-codesign-bin", fakeCodesign(t))
	if code != 0 || !strings.Contains(out, "IDENTICAL") || !strings.Contains(out, "tcc-reset' for this check") {
		t.Fatalf("code %d: %s", code, out)
	}
}

func TestDiffer(t *testing.T) {
	code, out := runArgs("-make-bin", "/a/one", "-bazel-bin", "/b/odd", "-codesign-bin", fakeCodesign(t))
	if code != 1 || !strings.Contains(out, "DIFFER") {
		t.Fatalf("code %d: %s", code, out)
	}
}

func TestUsage(t *testing.T) {
	if code, _ := runArgs(); code != 2 {
		t.Fatalf("code %d, want 2", code)
	}
}
