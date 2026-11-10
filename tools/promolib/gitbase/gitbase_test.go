package gitbase_test

import (
	"errors"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/tools/promolib/gitbase"
	"github.com/tstapler/stapler-squad/tools/promolib/gitbase/gitbasetest"
)

func TestResolveBase_SingleParentFailsLoudly(t *testing.T) {
	t.Parallel()
	dir := gitbasetest.LinearRepo(t, gitbasetest.Files{"a.txt": []byte("a")})
	r, err := gitbase.Open(dir)
	require.NoError(t, err)

	_, err = r.Changes("")
	var wrong *gitbase.WrongBaseError
	require.ErrorAs(t, err, &wrong)
	assert.Equal(t, 0, wrong.Parents)
	assert.Contains(t, err.Error(), "-base")

	_, err = r.ReadBlob(gitbase.DefaultBase, "a.txt")
	require.ErrorAs(t, err, &wrong)
}

func TestChanges_MergeCommitYieldsPRChangesOnly(t *testing.T) {
	t.Parallel()
	dir := gitbasetest.PRRepo(t,
		gitbasetest.Files{"keep.txt": []byte("k"), "mod.txt": []byte("1"), "gone.txt": []byte("g")},
		gitbasetest.Files{"mod.txt": []byte("22"), "gone.txt": nil, "new/x.webm": []byte("12345")})
	r, err := gitbase.Open(dir)
	require.NoError(t, err)

	ch, err := r.Changes("")
	require.NoError(t, err)
	sort.Slice(ch.Changed, func(i, j int) bool { return ch.Changed[i].Path < ch.Changed[j].Path })
	assert.Equal(t, []gitbase.FileChange{
		{Path: "mod.txt", Size: 2, Added: false},
		{Path: "new/x.webm", Size: 5, Added: true},
	}, ch.Changed)
	assert.Equal(t, []string{"gone.txt"}, ch.Deleted)
}

func TestReadBlob_ReturnsBaseContentNotPRContent(t *testing.T) {
	t.Parallel()
	dir := gitbasetest.PRRepo(t,
		gitbasetest.Files{"gate.json": []byte("old")},
		gitbasetest.Files{"gate.json": []byte("new"), "added.txt": []byte("x")})
	r, err := gitbase.Open(dir)
	require.NoError(t, err)

	got, err := r.ReadBlob("", "gate.json")
	require.NoError(t, err)
	assert.Equal(t, "old", string(got))

	_, err = r.ReadBlob("", "added.txt")
	require.Error(t, err)
	assert.True(t, errors.Is(err, gitbase.ErrNotFound))

	_, err = r.ReadBlob("", "no/such/dir/file")
	assert.True(t, errors.Is(err, gitbase.ErrNotFound))
}

func TestFiles_ListsIndex(t *testing.T) {
	t.Parallel()
	dir := gitbasetest.LinearRepo(t, gitbasetest.Files{"a.txt": []byte("a"), "d/b.txt": []byte("b")})
	r, err := gitbase.Open(dir)
	require.NoError(t, err)
	files, err := r.Files()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a.txt", "d/b.txt"}, files)
}

func TestMergeBaseWith(t *testing.T) {
	t.Parallel()
	dir := gitbasetest.PRRepo(t, gitbasetest.Files{"a": []byte("1")}, gitbasetest.Files{"a": []byte("2")})
	r, err := gitbase.Open(dir)
	require.NoError(t, err)
	got, err := r.MergeBaseWith("HEAD^1")
	require.NoError(t, err)
	assert.Len(t, got, 40)
}

func TestListAt_ListsDirectoryAtBaseAndMissingIsEmpty(t *testing.T) {
	t.Parallel()
	dir := gitbasetest.PRRepo(t,
		gitbasetest.Files{"d/a.json": []byte("1"), "d/sub/b.json": []byte("2"), "other.txt": []byte("3")},
		gitbasetest.Files{"d/new.json": []byte("4")})
	r, err := gitbase.Open(dir)
	require.NoError(t, err)

	got, err := r.ListAt("", "d")
	require.NoError(t, err)
	sort.Strings(got)
	assert.Equal(t, []string{"d/a.json", "d/sub/b.json"}, got)

	none, err := r.ListAt("", "absent")
	require.NoError(t, err)
	assert.Empty(t, none)
}
