package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeBin writes a shell script that answers `version` and carries payload bytes after the script.
func writeBin(t *testing.T, dir, versionLine, payload string) string {
	t.Helper()
	p := filepath.Join(dir, "stapler-squad")
	body := "#!/bin/sh\n[ \"$1\" = version ] && echo '" + versionLine + "'\nexit 0\n# " + payload + "\n"
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func runArgs(args ...string) (int, string) {
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
	return code, out.String() + errb.String()
}

func TestPass(t *testing.T) {
	d := t.TempDir()
	bin := writeBin(t, d, "stapler-squad version 1.2.3", "TMUXBYTES TYMUXDBYTES")
	tmux, tymuxd := write(t, d, "tmux", "TMUXBYTES"), write(t, d, "tymuxd", "TYMUXDBYTES")
	code, out := runArgs("-bin", bin, "-tmux", tmux, "-tymuxd", tymuxd)
	if code != 0 || !strings.Contains(out, "embedded tmux: ok") || !strings.Contains(out, "version: ok (1.2.3)") {
		t.Fatalf("code %d: %s", code, out)
	}
}

func TestVersionAfterLogNoise(t *testing.T) {
	bin := writeBin(t, t.TempDir(), `{"level":"INFO","msg":"trace provider initialized"}'; echo 'stapler-squad version 4.5.6`, "")
	if code, out := runArgs("-bin", bin); code != 0 || !strings.Contains(out, "version: ok (4.5.6)") {
		t.Fatalf("code %d: %s", code, out)
	}
}

func TestFailures(t *testing.T) {
	d := t.TempDir()
	good := writeBin(t, d, "stapler-squad version 1.2.3", "TMUXBYTES")
	cases := map[string]struct {
		args []string
		want string
	}{
		"empty version": {[]string{"-bin", writeBin(t, t.TempDir(), "stapler-squad version ", "")}, "no \"stapler-squad version <v>\" line"},
		"dev version":   {[]string{"-bin", writeBin(t, t.TempDir(), "stapler-squad version dev", "")}, "unstamped"},
		"tmux mismatch": {[]string{"-bin", good, "-tmux", write(t, d, "other", "DIFFERENT")}, "tmux"},
		"missing bin":   {[]string{"-bin", filepath.Join(d, "nope")}, "no such file"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			code, out := runArgs(tc.args...)
			if code != 1 || !strings.Contains(out, tc.want) {
				t.Fatalf("code %d, want 1 containing %q: %s", code, tc.want, out)
			}
		})
	}
}

func TestAllowDev(t *testing.T) {
	bin := writeBin(t, t.TempDir(), "stapler-squad version dev", "")
	if code, out := runArgs("-bin", bin, "-allow-dev"); code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
}

func TestSymlinkAndBazelPathRejected(t *testing.T) {
	d := t.TempDir()
	real := writeBin(t, d, "stapler-squad version 1.2.3", "")
	link := filepath.Join(d, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if code, out := runArgs("-bin", link); code != 1 || !strings.Contains(out, "symlink") {
		t.Fatalf("symlink: code %d: %s", code, out)
	}
	bz := filepath.Join(d, "bazel-bin")
	if err := os.Mkdir(bz, 0o755); err != nil {
		t.Fatal(err)
	}
	inBz := writeBin(t, bz, "stapler-squad version 1.2.3", "")
	if code, out := runArgs("-bin", inBz); code != 1 || !strings.Contains(out, "Bazel output tree") {
		t.Fatalf("bazel path: code %d: %s", code, out)
	}
}

func TestCheckoutNamedBazelIsFine(t *testing.T) {
	d := filepath.Join(t.TempDir(), "bazel-epic9-install")
	if err := os.Mkdir(d, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := writeBin(t, d, "stapler-squad version 1.2.3", "")
	if code, out := runArgs("-bin", bin); code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
}

func TestCodesign(t *testing.T) {
	d := t.TempDir()
	bin := writeBin(t, d, "stapler-squad version 1.2.3", "")
	fake := write(t, d, "codesign", "#!/bin/sh\necho 'designated => anchor H\"abc\"'\n")
	if err := os.Chmod(fake, 0o755); err != nil {
		t.Fatal(err)
	}
	doc := func(dr string) string {
		return write(t, d, "doc.md", "<!-- designated-requirement:begin -->\n"+dr+"\n<!-- designated-requirement:end -->\n")
	}
	if code, out := runArgs("-bin", bin, "-codesign", "-codesign-bin", fake, "-dr-doc", doc(`designated => anchor H"abc"`)); code != 0 {
		t.Fatalf("match: code %d: %s", code, out)
	}
	if code, out := runArgs("-bin", bin, "-codesign", "-codesign-bin", fake, "-dr-doc", doc(`designated => anchor H"zzz"`)); code != 1 || !strings.Contains(out, "differs") {
		t.Fatalf("mismatch: code %d: %s", code, out)
	}
	if code, out := runArgs("-bin", bin, "-codesign", "-codesign-bin", fake, "-dr-doc", doc("(not recorded)")); code != 1 || !strings.Contains(out, "no designated requirement recorded") {
		t.Fatalf("unrecorded: code %d: %s", code, out)
	}
}

func TestUsage(t *testing.T) {
	if code, _ := runArgs(); code != 2 {
		t.Fatalf("code %d, want 2", code)
	}
}
