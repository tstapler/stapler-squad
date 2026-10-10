package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
		"color.textPrimary":"image-set(\"https://evil/p\" 1x)",
		"color.textMuted":"url(http://evil)",
		"color.borderColor":"rgba(229,9,20,0.35)",
		"shadow.sm":"0 1px 2px 0 rgba(0,0,0,0.3)",
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
	assert.Equal(t, map[string]string{
		"color.primary": "#e50914", "color.statusDot.running": "#46d369",
		"color.borderColor": "rgba(229,9,20,0.35)", "shadow.sm": "0 1px 2px 0 rgba(0,0,0,0.3)",
	}, got[0].Tokens)
	assert.Equal(t, "nobase", got[1].ID)
	assert.Equal(t, defaultThemeBase, got[1].Base)
}

func TestLoadUserThemes_should_ReturnEmpty_When_DirMissing(t *testing.T) {
	assert.Empty(t, loadUserThemes(filepath.Join(t.TempDir(), "nope")))
}

func TestLoadUserThemes_should_CapFilesAndTokens(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < maxUserThemes+5; i++ {
		name := fmt.Sprintf("t%03d.json", i)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(`{}`), 0o600))
	}
	assert.Len(t, loadUserThemes(dir), maxUserThemes)

	tokens := map[string]string{}
	for i := 0; i < maxThemeTokens+10; i++ {
		tokens[fmt.Sprintf("color.t%s", strings.Repeat("a", i%20+1)+fmt.Sprint(i))] = "#fff"
	}
	got := sanitizeUserTheme("big", userTheme{Tokens: tokens})
	assert.LessOrEqual(t, len(got.Tokens), maxThemeTokens)
}
