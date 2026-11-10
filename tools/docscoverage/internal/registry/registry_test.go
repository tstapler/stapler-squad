package registry_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/model"
	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/registry"
)

const repoRoot = "../../../.."

func TestLoadsArrayFileAndDuplicateLauncherPresets(t *testing.T) {
	t.Parallel()
	files := registry.Files{
		"docs/registry/features/escape-analytics.json":             []byte(`[{"id":"escape:query","type":"backend"},{"id":"escape:summary","type":"backend"}]`),
		"docs/registry/features/backend/launcher-presets/get.json": []byte(`{"id":"launcher_presets:get","type":"backend"}`),
		"docs/registry/features/backend/launcher_presets/get.json": []byte(`{"id":"launcher_presets:get","type":"backend"}`),
		"docs/registry/features/frontend/x.json":                   []byte(`{"id":"x","type":"frontend","name":"X"}`),
	}
	reg, err := registry.Parse(files)
	require.NoError(t, err)
	assert.Len(t, reg.Features, 4)
	require.Len(t, reg.Warnings, 1)
	assert.Contains(t, reg.Warnings[0], "launcher-presets/get.json")
	assert.Contains(t, reg.Warnings[0], "launcher_presets/get.json")
	assert.True(t, reg.Features["escape:query"].DefaultUserFacing(), "root-dir entries default user facing whatever their type")
	assert.False(t, reg.Features["launcher_presets:get"].DefaultUserFacing())
}

func TestParseRejectsEntryWithoutID(t *testing.T) {
	t.Parallel()
	_, err := registry.Parse(registry.Files{"docs/registry/features/a.json": []byte(`{"name":"x"}`)})
	require.ErrorContains(t, err, "without an id")
}

func TestSidecarFilenameEncoding(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "alias%3Ainvoke.json", registry.SidecarFilename("alias:invoke"))
	assert.Equal(t, "a%2Fb%25c.json", registry.SidecarFilename("a/b%c"))
	assert.Equal(t, "plain-id.json", registry.SidecarFilename("plain-id"))
}

func TestSidecarEncodeDecodeRoundTripAllIds(t *testing.T) {
	t.Parallel()
	files, err := registry.ReadDir(repoRoot, registry.FeaturesDir)
	require.NoError(t, err)
	reg, err := registry.Parse(files)
	require.NoError(t, err)
	require.Greater(t, len(reg.Features), 400, "the real registry should be loaded")
	seen := map[string]model.FeatureID{}
	colons := 0
	for _, id := range reg.IDs() {
		name := registry.SidecarFilename(id)
		back, err := registry.DecodeSidecarFilename(name)
		require.NoError(t, err)
		assert.Equal(t, id, back)
		if prev, dup := seen[name]; dup {
			t.Fatalf("ids %q and %q encode to the same file %s", prev, id, name)
		}
		seen[name] = id
		if len(name) > 0 && name != string(id)+".json" {
			colons++
		}
	}
	assert.Positive(t, colons)
}

func TestParseSidecarsFlagsMisnamedAndDuplicate(t *testing.T) {
	t.Parallel()
	sc := registry.ParseSidecars(registry.Files{
		"docs/registry/docs-coverage/alias:invoke.json":   []byte(`{"id":"alias:invoke"}`),
		"docs/registry/docs-coverage/alias%3Amanage.json": []byte(`{"id":"alias:manage","status":"grandfathered"}`),
		"docs/registry/docs-coverage/schema.json":         []byte(`{}`),
		"docs/registry/docs-coverage/bad.json":            []byte(`{`),
	})
	assert.Len(t, sc.ByID, 2)
	require.Len(t, sc.Problems, 2)
	assert.Contains(t, sc.Problems[0]+sc.Problems[1], "must be stored as alias%3Ainvoke.json")
}
