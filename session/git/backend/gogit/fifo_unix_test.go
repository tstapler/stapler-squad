//go:build unix

package gogit

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFIFOIncludeDoesNotBlock(t *testing.T) {
	dir, _, _ := initRepo(t, 1)
	fifo := filepath.Join(t.TempDir(), "pipe")
	require.NoError(t, syscall.Mkfifo(fifo, 0o600))
	cfg := filepath.Join(dir, ".git", "config")
	existing, err := os.ReadFile(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfg, append(existing, []byte("[include]\n\tpath = "+fifo+"\n")...), 0o600))

	b := newBackend(t)
	done := make(chan error, 1)
	go func() {
		_, err := b.RepoRoot(context.Background(), local(dir))
		done <- err
	}()
	select {
	case err := <-done:
		assert.True(t, isFallback(err), "%v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("reading a FIFO include blocked")
	}
}
