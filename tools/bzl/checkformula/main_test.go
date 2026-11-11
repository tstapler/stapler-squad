package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const archive = "stapler-squad_1.0.0_linux_amd64.tar.gz"

func fixture(t *testing.T, content, sumsSHA, formulaSHA string) (dist, formula string) {
	t.Helper()
	dist = t.TempDir()
	if content != "" {
		if err := os.WriteFile(filepath.Join(dist, archive), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sum := sha256.Sum256([]byte("payload"))
	real := hex.EncodeToString(sum[:])
	if sumsSHA == "" {
		sumsSHA = real
	}
	if formulaSHA == "" {
		formulaSHA = real
	}
	write(t, filepath.Join(dist, "checksums.txt"), sumsSHA+"  "+archive+"\n")
	formula = filepath.Join(dist, "homebrew", "Formula", "stapler-squad.rb")
	if err := os.MkdirAll(filepath.Dir(formula), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, formula, `class StaplerSquad < Formula
  on_linux do
    if Hardware::CPU.intel?
      url "https://github.com/o/r/releases/download/v1.0.0/`+archive+`"
      sha256 "`+formulaSHA+`"
    end
  end
end
`)
	return dist, formula
}

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheck(t *testing.T) {
	t.Parallel()
	bad := strings.Repeat("0", 64)
	tests := []struct {
		name, content, sums, formula, wantProblem string
	}{
		{"match", "payload", "", "", ""},
		{"formula sha differs from checksums", "payload", "", bad, "!= checksums.txt"},
		{"checksums differ from archive", "payload", bad, bad, "!= archive digest"},
		{"archive missing from dist", "", "", "", "no such file"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dist, formula := fixture(t, tc.content, tc.sums, tc.formula)
			problems, n, err := check(dist, formula)
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(problems, "\n")
			if tc.wantProblem == "" && len(problems) != 0 {
				t.Fatalf("unexpected problems: %s", joined)
			}
			if tc.wantProblem != "" && !strings.Contains(joined, tc.wantProblem) {
				t.Fatalf("problems %q lack %q", joined, tc.wantProblem)
			}
			if n != 1 {
				t.Fatalf("checked %d assets, want 1", n)
			}
		})
	}
}

func TestAssetMissingFromFormula(t *testing.T) {
	t.Parallel()
	dist, formula := fixture(t, "payload", "", "")
	extra := "stapler-squad_1.0.0_darwin_arm64.tar.gz"
	f, err := os.OpenFile(filepath.Join(dist, "checksums.txt"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(strings.Repeat("a", 64) + "  " + extra + "\n")
	f.Close()
	problems, _, err := check(dist, formula)
	if err != nil || !strings.Contains(strings.Join(problems, "\n"), extra+": release asset missing") {
		t.Fatalf("problems=%v err=%v", problems, err)
	}
}

func TestUrlWithoutSHAAndNonReleaseURL(t *testing.T) {
	t.Parallel()
	_, problems := parseFormula(strings.NewReader("url \"https://example.com/x.tar.gz\"\n"))
	joined := strings.Join(problems, "\n")
	if !strings.Contains(joined, "not a GitHub release download") || !strings.Contains(joined, "has no sha256") {
		t.Fatalf("problems: %s", joined)
	}
}

func TestRunExitCodes(t *testing.T) {
	t.Parallel()
	dist, formula := fixture(t, "payload", "", "")
	var out, errb bytes.Buffer
	if code := run([]string{"-dist", dist}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	write(t, filepath.Join(dist, archive), "tampered")
	if code := run([]string{"-dist", dist, "-formula", formula}, &out, &errb); code != 1 {
		t.Fatalf("tampered archive: exit %d, want 1", code)
	}
	if code := run([]string{"-dist", filepath.Join(dist, "nope")}, &out, &errb); code != 2 {
		t.Fatalf("missing dist: exit %d, want 2", code)
	}
}
