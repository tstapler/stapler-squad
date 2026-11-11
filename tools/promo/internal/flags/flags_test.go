package flags_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/tools/promo/internal/flags"
	"github.com/tstapler/stapler-squad/tools/promolib/gitbase"
	"github.com/tstapler/stapler-squad/tools/promolib/gitbase/gitbasetest"
)

func TestRealFileHasIntroductionDefaults(t *testing.T) {
	t.Parallel()
	got, err := flags.FromTree(filepath.Join("..", "..", "..", "..", flags.Path))
	require.NoError(t, err)
	assert.Equal(t, flags.Flags{FreshnessEnforce: false, DocsCoverageEnforce: false, GuardsEnforce: true}, got)

	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", flags.Path))
	require.NoError(t, err)
	_, err = flags.Parse(raw)
	require.NoError(t, err)
}

func TestParse_RejectsMalformed(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, in string }{
		{"not json", "nope"},
		{"missing key", `{"freshnessEnforce": false, "docsCoverageEnforce": false}`},
		{"string instead of bool", `{"freshnessEnforce": "no", "docsCoverageEnforce": false, "guardsEnforce": true}`},
		{"null", `{"freshnessEnforce": null, "docsCoverageEnforce": false, "guardsEnforce": true}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := flags.Parse([]byte(tt.in))
			assert.Error(t, err)
		})
	}
}

func TestFromBase(t *testing.T) {
	t.Parallel()
	t.Run("absent in base yields defaults even when the PR adds it", func(t *testing.T) {
		t.Parallel()
		dir := gitbasetest.PRRepo(t, gitbasetest.Files{"a": []byte("a")},
			gitbasetest.Files{flags.Path: []byte(`{"freshnessEnforce": true, "docsCoverageEnforce": true, "guardsEnforce": false}`)})
		r, err := gitbase.Open(dir)
		require.NoError(t, err)
		got, err := flags.FromBase(r, "")
		require.NoError(t, err)
		assert.Equal(t, flags.Defaults(), got)
	})
	t.Run("PR flipping guardsEnforce is judged by the base value", func(t *testing.T) {
		t.Parallel()
		dir := gitbasetest.PRRepo(t,
			gitbasetest.Files{flags.Path: []byte(`{"freshnessEnforce": false, "docsCoverageEnforce": false, "guardsEnforce": true}`)},
			gitbasetest.Files{flags.Path: []byte(`{"freshnessEnforce": false, "docsCoverageEnforce": false, "guardsEnforce": false}`)})
		r, err := gitbase.Open(dir)
		require.NoError(t, err)
		got, err := flags.FromBase(r, "")
		require.NoError(t, err)
		assert.True(t, got.GuardsEnforce)
	})
	t.Run("present but malformed is an error", func(t *testing.T) {
		t.Parallel()
		dir := gitbasetest.PRRepo(t, gitbasetest.Files{flags.Path: []byte("{")}, gitbasetest.Files{"x": []byte("x")})
		r, err := gitbase.Open(dir)
		require.NoError(t, err)
		_, err = flags.FromBase(r, "")
		assert.Error(t, err)
	})
}

func TestFromTree_AbsentIsDefaults(t *testing.T) {
	t.Parallel()
	got, err := flags.FromTree(filepath.Join(t.TempDir(), "missing.json"))
	require.NoError(t, err)
	assert.Equal(t, flags.Defaults(), got)
}
