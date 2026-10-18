package services

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestSnapshotSDDPlanOutOfWorktree_should_ReplaceEarlierCopyAndClearStaleTemp(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "durable", "sdd-plan")
	writeFile(t, filepath.Join(src, "plan.md"), "new plan")
	writeFile(t, filepath.Join(dst, "old.md"), "from an earlier triage")
	writeFile(t, dst+".tmp/junk.md", "left by a crashed run")

	got, err := snapshotSDDPlanOutOfWorktree(src, dst)

	require.NoError(t, err)
	assert.Equal(t, dst, got)
	assert.FileExists(t, filepath.Join(dst, "plan.md"))
	assert.NoFileExists(t, filepath.Join(dst, "old.md"), "the earlier snapshot must be replaced, not merged")
	assert.NoDirExists(t, dst+".tmp")
}

func TestSnapshotSDDPlanOutOfWorktree_should_KeepPreviousSnapshotAndLeaveNoTemp_When_CopyFails(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "durable", "sdd-plan")
	writeFile(t, filepath.Join(dst, "plan.md"), "previous good plan")

	_, err := snapshotSDDPlanOutOfWorktree(filepath.Join(root, "missing-src"), dst)

	require.Error(t, err)
	prev, readErr := os.ReadFile(filepath.Join(dst, "plan.md"))
	require.NoError(t, readErr, "a failed copy must not destroy the previous good snapshot")
	assert.Equal(t, "previous good plan", string(prev))
	assert.NoDirExists(t, dst+".tmp")
}

func TestSnapshotSDDPlanOutOfWorktree_should_SkipSymlinks(t *testing.T) {
	root := t.TempDir()
	secret := filepath.Join(root, "secret.txt")
	writeFile(t, secret, "host secret")
	src := filepath.Join(root, "src")
	writeFile(t, filepath.Join(src, "plan.md"), "plan")
	require.NoError(t, os.Symlink(secret, filepath.Join(src, "leak.md")))
	require.NoError(t, os.Symlink(root, filepath.Join(src, "escape")))
	dst := filepath.Join(root, "durable", "sdd-plan")

	_, err := snapshotSDDPlanOutOfWorktree(src, dst)

	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dst, "plan.md"))
	assert.NoFileExists(t, filepath.Join(dst, "leak.md"))
	assert.NoDirExists(t, filepath.Join(dst, "escape"))
}

func TestSnapshotSDDPlanOutOfWorktree_should_Fail_When_SourceIsASymlink(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	writeFile(t, filepath.Join(real, "plan.md"), "plan")
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(real, link))

	_, err := snapshotSDDPlanOutOfWorktree(link, filepath.Join(root, "durable", "sdd-plan"))

	require.Error(t, err)
}

func TestSnapshotSDDPlanOutOfWorktree_should_Fail_When_TooManyFiles(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	for i := 0; i <= maxSDDPlanSnapshotFiles; i++ {
		writeFile(t, filepath.Join(src, fmt.Sprintf("f%d.md", i)), "x")
	}
	dst := filepath.Join(root, "durable", "sdd-plan")

	_, err := snapshotSDDPlanOutOfWorktree(src, dst)

	require.Error(t, err)
	assert.NoDirExists(t, dst)
	assert.NoDirExists(t, dst+".tmp")
}
