package gitconfig

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadRegularFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	require.NoError(t, os.WriteFile(file, []byte("hello"), 0o600))
	data, err := ReadRegularFile(file, 100)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))

	_, err = ReadRegularFile(file, 4)
	assert.ErrorIs(t, err, ErrTooLarge)
	_, err = ReadRegularFile(filepath.Join(dir, "missing"), 100)
	assert.ErrorIs(t, err, fs.ErrNotExist)
	_, err = ReadRegularFile(filepath.Join(file, "child"), 100)
	assert.ErrorIs(t, err, syscall.ENOTDIR)
	_, err = ReadRegularFile(dir, 100)
	assert.ErrorIs(t, err, ErrNotRegular)
	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink(file, link))
	data, err = ReadRegularFile(link, 100)
	require.NoError(t, err, "a symlink to a regular file is read")
	assert.Equal(t, "hello", string(data))
}

func TestReadRegularFileDoesNotBlockOnAFIFO(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("no mkfifo: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := ReadRegularFile(fifo, 100)
		done <- err
	}()
	select {
	case err := <-done:
		assert.True(t, errors.Is(err, ErrNotRegular), "got %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("ReadRegularFile blocked on a FIFO")
	}
}
