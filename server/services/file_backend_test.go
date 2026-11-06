package services

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// backendUnderTest builds the same tiny tree on both backends and returns each root.
func backendsWithTree(t *testing.T) map[string]FileRoot {
	t.Helper()
	mtime := time.Unix(1_700_000_000, 0)
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "sub"))
	writeFixtureFile(t, filepath.Join(dir, "sub", "f.txt"), []byte("hello"))
	require.NoError(t, os.Chtimes(filepath.Join(dir, "sub", "f.txt"), mtime, mtime))
	require.NoError(t, os.Symlink("sub/f.txt", filepath.Join(dir, "link")))
	osRoot, err := osRootBackend{}.OpenRoot(dir)
	require.NoError(t, err)

	mem := newMemBackend()
	mem.AddFile("/mem", "sub/f.txt", []byte("hello"), mtime)
	mem.AddSymlink("/mem", "link", "sub/f.txt")
	memRoot, err := mem.OpenRoot("/mem")
	require.NoError(t, err)

	roots := map[string]FileRoot{"osRoot": osRoot, "mem": memRoot}
	t.Cleanup(func() {
		for _, r := range roots {
			_ = r.Close()
		}
	})
	return roots
}

func TestFileBackend_ShouldBehaveTheSame_When_OsRootAndMemServeOneTree(t *testing.T) {
	for name, root := range backendsWithTree(t) {
		t.Run(name, func(t *testing.T) {
			info, err := root.Lstat("sub/f.txt")
			require.NoError(t, err)
			assert.Equal(t, int64(5), info.Size())
			assert.True(t, info.ModTime().Equal(time.Unix(1_700_000_000, 0)))
			assert.False(t, info.IsDir())

			dirInfo, err := root.Lstat("sub")
			require.NoError(t, err)
			assert.True(t, dirInfo.IsDir())

			linkInfo, err := root.Lstat("link")
			require.NoError(t, err)
			assert.NotZero(t, linkInfo.Mode()&os.ModeSymlink, "Lstat must not follow the final link")
			target, err := root.Readlink("link")
			require.NoError(t, err)
			assert.Equal(t, "sub/f.txt", target)

			h, err := root.Open("sub/f.txt")
			require.NoError(t, err)
			defer func() { _ = h.Close() }()
			buf := make([]byte, 3)
			n, err := h.ReadAt(buf, 1)
			require.NoError(t, err)
			assert.Equal(t, "ell", string(buf[:n]))
			st, err := h.Stat()
			require.NoError(t, err)
			assert.Equal(t, int64(5), st.Size())

			_, err = root.Lstat("nope")
			assert.True(t, os.IsNotExist(err))
			_, err = root.Open("nope")
			assert.True(t, os.IsNotExist(err))
		})
	}
}

func TestMemBackend_ShouldNotChangeClassification_When_ReadsComeBackShort(t *testing.T) {
	text := []byte(strings.Repeat("line of text\n", 1000))
	for _, short := range []int{0, 1, 7, 511} {
		mem := newMemBackend()
		mem.ShortRead = short
		mem.AddFile("/m", "a.txt", text, time.Unix(1, 0))
		root, err := mem.OpenRoot("/m")
		require.NoError(t, err)
		h, err := root.Open("a.txt")
		require.NoError(t, err)

		ct, binary := sniffContent(h, false)
		assert.False(t, binary, "short=%d", short)
		assert.True(t, strings.HasPrefix(ct, "text/plain"), "short=%d: %s", short, ct)
		got, truncated, err := readTextCapped(h, int64(len(text)))
		require.NoError(t, err)
		assert.False(t, truncated)
		assert.Equal(t, text, got, "short=%d", short)
		_ = h.Close()
	}
}

func TestMemBackend_ShouldSniffBinary_When_NulAfterShortReads(t *testing.T) {
	mem := newMemBackend()
	mem.ShortRead = 5
	mem.AddFile("/m", "x.dat", append([]byte("text then "), 0, 'x'), time.Unix(1, 0))
	root, _ := mem.OpenRoot("/m")
	h, err := root.Open("x.dat")
	require.NoError(t, err)
	_, binary := sniffContent(h, false)
	assert.True(t, binary)
	_, err = io.ReadAll(h)
	assert.NoError(t, err)
}
