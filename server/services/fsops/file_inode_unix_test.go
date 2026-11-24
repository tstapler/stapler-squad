//go:build !windows

package fsops

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileInode_ShouldReturnTheStatInode_And_ZeroWhenBackendHasNone(t *testing.T) {
	t.Parallel()
	p := filepath.Join(t.TempDir(), "f")
	require.NoError(t, os.WriteFile(p, nil, 0o600))
	info, err := os.Stat(p)
	require.NoError(t, err)

	assert.Equal(t, uint64(info.Sys().(*syscall.Stat_t).Ino), FileInode(info))
	assert.Zero(t, FileInode(memFileInfo{name: "x", n: &memNode{}}), "a backend without an inode reports 0")
}
