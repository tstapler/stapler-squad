package services

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// memFromDir replays an on-disk tree into a memBackend rooted at the same dir string.
func memFromDir(t *testing.T, dir string) *memBackend {
	t.Helper()
	mem := newMemBackend()
	mem.tree(dir) // an empty tree still opens
	require.NoError(t, filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == dir {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			target, lerr := os.Readlink(p)
			require.NoError(t, lerr)
			mem.AddSymlink(dir, rel, target)
		case d.IsDir():
			mem.tree(dir).mkParents(rel + "/x")
		default:
			data, rerr := os.ReadFile(p)
			require.NoError(t, rerr)
			info, ierr := d.Info()
			require.NoError(t, ierr)
			mem.AddFile(dir, rel, data, info.ModTime())
		}
		return nil
	}))
	return mem
}

// FileOpsConformance runs the shared FileOps cases against one backend. Story 1.6.2
// adds range cases and Story 2.1.5 adds patch cases here.
func FileOpsConformance(t *testing.T, newOps func(t *testing.T, dir string) *FileOps) {
	t.Helper()
	dir := buildReadCorpus(t)
	ops := newOps(t, dir)
	svc := &FileService{workspace: ops.policy.Workspace, ops: ops}

	t.Run("read-all and stat match the pre-refactor corpus byte for byte", func(t *testing.T) {
		raw, err := os.ReadFile(readGoldenPath)
		require.NoError(t, err)
		var want map[string]string
		require.NoError(t, json.Unmarshal(raw, &want))
		assert.Equal(t, want, readCorpusDigest(t, svc))
	})

	t.Run("capabilities advertise the read floor", func(t *testing.T) {
		caps := ops.Capabilities(context.Background())
		assert.Contains(t, caps.Ops, sessionv1.FileOp_FILE_OP_STAT)
		assert.Contains(t, caps.Ops, sessionv1.FileOp_FILE_OP_READ_ALL)
	})
}

func TestFileOpsConformance_ShouldPassOnOsRootBackendAndMemBackend(t *testing.T) {
	t.Run("osRootBackend", func(t *testing.T) {
		FileOpsConformance(t, func(t *testing.T, dir string) *FileOps {
			return NewFileOps(FilePolicy{Workspace: &fakeWorkspaceProvider{existingDir: dir}}, strictBackend())
		})
	})
	t.Run("memBackend", func(t *testing.T) {
		FileOpsConformance(t, func(t *testing.T, dir string) *FileOps {
			return NewFileOps(FilePolicy{Workspace: &fakeWorkspaceProvider{existingDir: dir}}, strictOver{memFromDir(t, dir)})
		})
	})
}
