package inbox

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

const sentinelToken = "tok-SENTINEL-9f3a"

func writeToken(t *testing.T, mode os.FileMode, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "inbox.token")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadToken_should_ReturnTrimmedToken_When_FileIsRegular0600(t *testing.T) {
	path := writeToken(t, 0o600, sentinelToken+"\n")
	got, err := ReadToken(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != sentinelToken {
		t.Fatalf("got %q, want %q", got, sentinelToken)
	}

	// No caching: a rotated token is seen on the next read.
	if err := os.WriteFile(path, []byte("tok-rotated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, _ = ReadToken(path); got != "tok-rotated" {
		t.Fatalf("got %q after rotation", got)
	}
}

func TestReadToken_should_Refuse_When_FileIsGroupOrWorldAccessible(t *testing.T) {
	for _, mode := range []os.FileMode{0o640, 0o604, 0o644, 0o660, 0o666} {
		t.Run(mode.String(), func(t *testing.T) {
			path := writeToken(t, mode, sentinelToken)
			got, err := ReadToken(path)
			if !errors.Is(err, ErrTokenPermissions) {
				t.Fatalf("err = %v, want ErrTokenPermissions", err)
			}
			if got != "" || strings.Contains(err.Error(), sentinelToken) {
				t.Fatal("token value leaked")
			}
			if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), mode.String()) {
				t.Fatalf("error should name path and mode: %v", err)
			}
		})
	}
}

func TestReadToken_should_Refuse_When_PathIsSymlinkDirectoryOrFIFO(t *testing.T) {
	dir := t.TempDir()
	real := writeToken(t, 0o600, sentinelToken)

	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	subdir := filepath.Join(dir, "sub")
	if err := os.Mkdir(subdir, 0o600); err != nil {
		t.Fatal(err)
	}

	for name, path := range map[string]string{"symlink": link, "fifo": fifo, "directory": subdir} {
		t.Run(name, func(t *testing.T) {
			_, err := ReadToken(path)
			if !errors.Is(err, ErrTokenPermissions) {
				t.Fatalf("err = %v, want ErrTokenPermissions", err)
			}
			if strings.Contains(err.Error(), sentinelToken) {
				t.Fatal("token value leaked")
			}
		})
	}
}

func TestReadToken_should_ReturnMissingError_When_FileAbsentOrEmpty(t *testing.T) {
	if _, err := ReadToken(filepath.Join(t.TempDir(), "absent")); !errors.Is(err, ErrTokenMissing) {
		t.Fatalf("absent: err = %v, want ErrTokenMissing", err)
	}
	empty := writeToken(t, 0o600, " \n\t\n")
	if _, err := ReadToken(empty); !errors.Is(err, ErrTokenMissing) {
		t.Fatalf("whitespace-only: err = %v, want ErrTokenMissing", err)
	}
}

func TestReadToken_should_ExpandTildeAndHonorConfiguredPath_When_PathConfigured(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	def := filepath.Join(home, ".local/state/gh-signal/inbox.token")
	if err := os.MkdirAll(filepath.Dir(def), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(def, []byte("tok-default\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"", DefaultTokenPath} {
		if got, err := ReadToken(p); err != nil || got != "tok-default" {
			t.Fatalf("path %q: got %q, %v", p, got, err)
		}
	}

	custom := writeToken(t, 0o600, "tok-custom")
	if got, err := ReadToken(custom); err != nil || got != "tok-custom" {
		t.Fatalf("configured path: got %q, %v", got, err)
	}
}

func TestReadToken_should_LeaveFileUntouched_When_ReadCompletes(t *testing.T) {
	path := writeToken(t, 0o600, sentinelToken+"\n")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeBytes, _ := os.ReadFile(path)

	for range 3 {
		if _, err := ReadToken(path); err != nil {
			t.Fatal(err)
		}
	}

	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	afterBytes, _ := os.ReadFile(path)
	if after.Mode() != before.Mode() || !after.ModTime().Equal(before.ModTime()) ||
		after.Size() != before.Size() || !bytes.Equal(afterBytes, beforeBytes) {
		t.Fatal("token file was modified by reads")
	}
}
