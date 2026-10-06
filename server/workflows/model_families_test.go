package workflows

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ResolveModel ---

func TestResolveModel_KnownFamily_ExpectConcreteID(t *testing.T) {
	families := DefaultModelFamilies()
	resolved, err := ResolveModel(families, "family:sonnet")
	require.NoError(t, err)
	assert.Equal(t, families["sonnet"], resolved)
}

func TestResolveModel_LiteralModelID_ExpectPassThroughUnchanged(t *testing.T) {
	families := DefaultModelFamilies()
	resolved, err := ResolveModel(families, "claude-opus-4-8")
	require.NoError(t, err)
	assert.Equal(t, "claude-opus-4-8", resolved)
}

func TestResolveModel_Empty_ExpectPassThroughUnchanged(t *testing.T) {
	resolved, err := ResolveModel(DefaultModelFamilies(), "")
	require.NoError(t, err)
	assert.Equal(t, "", resolved)
}

func TestResolveModel_BareFamilyNameWithoutPrefix_ExpectNotResolved(t *testing.T) {
	// A pre-existing workflow that historically stored the literal string
	// "sonnet" (before family aliases existed) must not be silently
	// reinterpreted as the "sonnet" family.
	resolved, err := ResolveModel(DefaultModelFamilies(), "sonnet")
	require.NoError(t, err)
	assert.Equal(t, "sonnet", resolved)
}

func TestResolveModel_UnknownFamily_ExpectError(t *testing.T) {
	_, err := ResolveModel(DefaultModelFamilies(), "family:retired-name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retired-name")
}

// --- LoadModelFamilyOverride ---

func TestLoadModelFamilyOverride_ValidJSON_ExpectMergedOverDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"sonnet":"claude-sonnet-9000"}`), 0o600))

	families, err := LoadModelFamilyOverride(path)
	require.NoError(t, err)
	assert.Equal(t, "claude-sonnet-9000", families["sonnet"])
	// Unrelated defaults survive untouched.
	assert.Equal(t, DefaultModelFamilies()["opus"], families["opus"])
}

func TestLoadModelFamilyOverride_MissingFile_ExpectError(t *testing.T) {
	_, err := LoadModelFamilyOverride(filepath.Join(t.TempDir(), "does-not-exist.json"))
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestLoadModelFamilyOverride_MalformedJSON_ExpectError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	require.NoError(t, os.WriteFile(path, []byte(`{not valid json`), 0o600))

	_, err := LoadModelFamilyOverride(path)
	require.Error(t, err)
}

// --- ValidateModel ---

func TestValidateModel_Empty_ExpectValid(t *testing.T) {
	assert.NoError(t, ValidateModel(""))
}

func TestValidateModel_ValidLiteralID_ExpectValid(t *testing.T) {
	assert.NoError(t, ValidateModel("claude-sonnet-4-6"))
}

func TestValidateModel_ValidFamilyAlias_ExpectValid(t *testing.T) {
	assert.NoError(t, ValidateModel("family:sonnet"))
}

func TestValidateModel_EmbeddedSpace_ExpectRejected(t *testing.T) {
	assert.Error(t, ValidateModel("family: sonnet"))
}

func TestValidateModel_ShellMetacharacters_ExpectRejected(t *testing.T) {
	for _, bad := range []string{"claude-sonnet-4-6; rm -rf /", "$(whoami)", "model`id`", "a|b", "a&&b"} {
		assert.Error(t, ValidateModel(bad), "expected %q to be rejected", bad)
	}
}

func TestLoadModelFamilyOverride_InvalidModelID_ExpectError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"sonnet":"claude-sonnet-4-6; rm -rf /"}`), 0o600))

	_, err := LoadModelFamilyOverride(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sonnet")
}
