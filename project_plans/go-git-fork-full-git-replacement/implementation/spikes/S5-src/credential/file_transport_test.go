package credential

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport/server"
)

func TestFileTransport_should_SpawnGitUploadPack_NotGit_When_UploadPackOnPATH(t *testing.T) {
	sh := newShim(t)
	root, head := seedBare(t)
	repo, err := git.PlainClone(t.TempDir(), false, &git.CloneOptions{URL: "file://" + filepath.Join(root, "repo.git")})
	if err != nil {
		t.Fatal(err)
	}
	if h, _ := repo.Head(); h.Hash().String() != head {
		t.Fatal("head mismatch")
	}
	sp := sh.Spawns()
	t.Logf("spawns: %q", sp)
	if len(sp) != 1 || !strings.HasPrefix(sp[0], "git-upload-pack ") {
		t.Fatalf("expected exactly one git-upload-pack spawn, got %v", sp)
	}
}

func TestFileTransport_should_SpawnGitExecPath_When_UploadPackNotOnPATH(t *testing.T) {
	sh := newShim(t)
	// PATH with only the `git` shim: LookPath("git-upload-pack") fails -> go-git runs `git --exec-path`.
	only := t.TempDir()
	shimGit, _ := os.ReadFile(filepath.Join(filepath.Dir(sh.log), "git"))
	os.WriteFile(filepath.Join(only, "git"), shimGit, 0o755)
	t.Setenv("PATH", only)
	root, _ := seedBare(t)
	_, err := git.PlainClone(t.TempDir(), false, &git.CloneOptions{URL: "file://" + filepath.Join(root, "repo.git")})
	if err != nil {
		t.Fatal(err)
	}
	sp := sh.Spawns()
	t.Logf("spawns: %q", sp)
	if len(sp) != 1 || sp[0] != "git --exec-path" {
		t.Fatalf("expected `git --exec-path`, got %v", sp)
	}
}

func TestFileTransport_should_SpawnNothing_When_InProcessServerInstalledForFile(t *testing.T) {
	restoreProtocols(t)
	sh := newShim(t)
	root, head := seedBare(t)
	Install2 := func() { // in-process upload-pack/receive-pack served straight from the filesystem
		clientFor := server.NewClient(server.NewFilesystemLoader(osfs.New("/")))
		installFile(clientFor)
	}
	Install2()
	dir := t.TempDir()
	repo, err := git.PlainClone(dir, false, &git.CloneOptions{URL: "file://" + filepath.Join(root, "repo.git")})
	if err != nil {
		t.Fatal(err)
	}
	if h, _ := repo.Head(); h.Hash().String() != head {
		t.Fatal("head mismatch")
	}
	// push back through the same in-process transport
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("c\n"), 0o644)
	runGit(t, dir, "add", "c.txt")
	runGit(t, dir, "commit", "-qm", "c")
	spawnsBeforeOracle := len(sh.Spawns()) // runGit uses the real binary directly, not via shim
	if err := repo.Push(&git.PushOptions{}); err != nil {
		t.Fatalf("push: %v", err)
	}
	if got, want := runGit(t, filepath.Join(root, "repo.git"), "rev-parse", "main"), runGit(t, dir, "rev-parse", "HEAD"); got != want {
		t.Fatalf("push did not land: %s != %s", got, want)
	}
	if sp := sh.Spawns(); len(sp) != 0 || spawnsBeforeOracle != 0 {
		t.Fatalf("expected zero spawns, got %v", sp)
	}
}
