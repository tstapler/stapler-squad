package services

import (
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func TestFileServiceListFiles_ShouldHideAtomicTempFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".a.txt.ssq-tmp-dead"), []byte("x"), 0o644))
	svc := NewFileService(&fakeWorkspaceProvider{existingDir: dir})

	resp, err := svc.ListFiles(t.Context(), connect.NewRequest(&sessionv1.ListFilesRequest{SessionId: "test-session", IncludeIgnored: true}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.Files, 1)
	assert.Equal(t, "a.txt", resp.Msg.Files[0].Name)
}
