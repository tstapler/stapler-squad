package localtoken

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadOrCreate_should_CreateOwnerOnlyFileAndReuseIt(t *testing.T) {
	path := Path(t.TempDir())
	first, err := LoadOrCreate(path)
	require.NoError(t, err)
	assert.Len(t, first, 64)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	second, err := LoadOrCreate(path)
	require.NoError(t, err)
	assert.Equal(t, first, second)
}

func TestRead_should_Reject_When_FileReadableByOthers(t *testing.T) {
	path := filepath.Join(t.TempDir(), fileName)
	require.NoError(t, os.WriteFile(path, []byte("abc\n"), 0644))
	_, err := Read(path)
	assert.ErrorContains(t, err, "accessible by other users")
}

func TestRead_should_WrapNotExist_When_FileMissing(t *testing.T) {
	_, err := Read(filepath.Join(t.TempDir(), "missing"))
	assert.ErrorIs(t, err, fs.ErrNotExist)
	assert.Empty(t, FromConfigDir(t.TempDir()))
}

func TestCurlHeaderArg_should_QuotePathAndNeverEmbedToken(t *testing.T) {
	arg := CurlHeaderArg("/tmp/it's/token")
	assert.Contains(t, arg, `$(cat '/tmp/it'\''s/token' 2>/dev/null)`)
	assert.True(t, strings.HasPrefix(arg, "-H "))
}
