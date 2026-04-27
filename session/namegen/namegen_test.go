package namegen_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/namegen"
)

func TestGenerate_Format(t *testing.T) {
	re := regexp.MustCompile(`^\d{8}-[a-z]+-[a-z]+-\d{2}$`)
	for i := 0; i < 1000; i++ {
		name := namegen.Generate()
		assert.Regexp(t, re, name, "iteration %d: name %q does not match format", i, name)
		assert.LessOrEqual(t, len(name), 30, "name must be ≤ 30 chars, got %d: %s", len(name), name)
	}
}

func TestGenerate_ShellSafe(t *testing.T) {
	re := regexp.MustCompile(`^[a-z0-9-]+$`)
	for i := 0; i < 1000; i++ {
		name := namegen.Generate()
		assert.Regexp(t, re, name, "iteration %d: name %q contains unsafe chars", i, name)
		assert.NotEmpty(t, name)
	}
}

func TestGenerate_DatePrefix(t *testing.T) {
	name := namegen.Generate()
	dateStr := name[:8]
	_, err := time.Parse("20060102", dateStr)
	assert.NoError(t, err, "first 8 chars %q must be a valid date", dateStr)
	assert.Equal(t, time.Now().Format("20060102"), dateStr)
}

func TestGenerate_NumberRange(t *testing.T) {
	re := regexp.MustCompile(`-(\d{2})$`)
	for i := 0; i < 1000; i++ {
		name := namegen.Generate()
		m := re.FindStringSubmatch(name)
		require.Len(t, m, 2, "should have 2-digit suffix in %q", name)
		n, err := strconv.Atoi(m[1])
		require.NoError(t, err)
		assert.GreaterOrEqual(t, n, 0)
		assert.LessOrEqual(t, n, 99)
	}
}

func TestGenerateAndCreate_CreatesDir(t *testing.T) {
	path, err := namegen.GenerateAndCreate(t.TempDir(), 10)
	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err, "returned path must exist")
	assert.True(t, info.IsDir(), "returned path must be a directory")
}

func TestGenerateAndCreate_BaseDir_Created(t *testing.T) {
	nested := filepath.Join(t.TempDir(), "nested", "subdir")
	path, err := namegen.GenerateAndCreate(nested, 10)
	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestGenerateAndCreate_BaseDir_IsFile_Error(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "notadir")
	require.NoError(t, os.WriteFile(filePath, []byte("data"), 0644))

	_, err := namegen.GenerateAndCreate(filePath, 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}

func TestGenerateAndCreate_RetryOnCollision(t *testing.T) {
	baseDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(baseDir, "20260424-brave-falcon-01"), 0755))

	callCount := 0
	stubFn := func() string {
		callCount++
		if callCount <= 3 {
			return "20260424-brave-falcon-01"
		}
		return "20260424-calm-otter-42"
	}

	path, err := namegen.GenerateAndCreateWithFn(baseDir, 10, stubFn)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(baseDir, "20260424-calm-otter-42"), path)
	info, err := os.Stat(path)
	require.NoError(t, err, "generated directory must exist on disk")
	assert.True(t, info.IsDir())
	assert.Equal(t, 4, callCount)
}

func TestGenerateAndCreate_ErrorAfterMaxAttempts(t *testing.T) {
	baseDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(baseDir, "20260424-stuck-owl-00"), 0755))

	callCount := 0
	stubFn := func() string {
		callCount++
		return "20260424-stuck-owl-00"
	}

	path, err := namegen.GenerateAndCreateWithFn(baseDir, 10, stubFn)
	assert.Error(t, err)
	assert.Empty(t, path)
	assert.Equal(t, 10, callCount)
	assert.Contains(t, err.Error(), "failed to generate unique")
}

func TestWordLists_MinimumSize(t *testing.T) {
	adjs, nouns := namegen.ExportedWordLists()
	assert.GreaterOrEqual(t, len(adjs), 50, "need ≥ 50 adjectives")
	assert.GreaterOrEqual(t, len(nouns), 50, "need ≥ 50 nouns")

	re := regexp.MustCompile(`^[a-z]+$`)
	for _, w := range adjs {
		assert.Regexp(t, re, w, "adjective %q must be lowercase letters only", w)
	}
	for _, w := range nouns {
		assert.Regexp(t, re, w, "noun %q must be lowercase letters only", w)
	}
}
