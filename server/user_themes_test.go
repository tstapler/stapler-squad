package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadUserThemes_should_SanitizeAndSkipInvalid(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	write("netflix.json", `{"label":"Netflix","base":"dark","tokens":{
		"color.primary":"#e50914",
		"color.statusDot.running":"#46d369",
		"color.background":"red; background:url(http://evil)",
		"bad key":"#fff"}}`)
	write("nobase.json", `{"base":"bogus","tokens":{}}`)
	write("broken.json", `{not json`)
	write("Bad_ID.json", `{}`)
	write("notes.txt", `{}`)

	got := loadUserThemes(dir)

	require.Len(t, got, 2)
	assert.Equal(t, "netflix", got[0].ID)
	assert.Equal(t, "Netflix", got[0].Label)
	assert.Equal(t, "dark", got[0].Base)
	assert.Equal(t, map[string]string{"color.primary": "#e50914", "color.statusDot.running": "#46d369"}, got[0].Tokens)
	assert.Equal(t, "nobase", got[1].ID)
	assert.Equal(t, defaultThemeBase, got[1].Base)
}

func TestLoadUserThemes_should_ReturnEmpty_When_DirMissing(t *testing.T) {
	assert.Empty(t, loadUserThemes(filepath.Join(t.TempDir(), "nope")))
}
