package main

import (
	"bytes"
	"strings"
	"testing"
)

func seeded(failing string) Runner {
	return func(cmd []string) (string, int) {
		if strings.Contains(strings.Join(cmd, " "), failing) {
			return "diff --git a/x b/x", 1
		}
		return "", 0
	}
}

func TestEachDriftNamesItsRemedy(t *testing.T) {
	cases := map[string]string{"-mode=diff": "Run: make gazelle", "mod tidy": "Run: bazel mod tidy", "gen-freshness": "run: make sync"}
	for failing, remedy := range cases {
		fs := Check(seeded(failing), Gates())
		if len(fs) != 1 || !strings.Contains(fs[0], remedy) || !strings.Contains(fs[0], "diff --git") {
			t.Errorf("%s: %v", failing, fs)
		}
	}
}

func TestAllFreshExitsZero(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run(nil, seeded("never-matches"), &out, &errb); code != 0 || !strings.Contains(out.String(), "3 gate(s) fresh") {
		t.Fatalf("%d %s %s", code, out.String(), errb.String())
	}
}

func TestFailureExitsOneAndUnknownGateIsUsageError(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"gazelle"}, seeded("-mode=diff"), &out, &errb); code != 1 || !strings.Contains(errb.String(), "Run: make gazelle") {
		t.Fatalf("%d %s", code, errb.String())
	}
	if code := run([]string{"nope"}, seeded(""), &out, &errb); code != 2 {
		t.Fatalf("%d", code)
	}
}
