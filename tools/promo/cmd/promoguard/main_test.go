package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/tools/promolib/gitbase/gitbasetest"
)

func TestRun_Dispatch(t *testing.T) {
	t.Parallel()
	dir := gitbasetest.PRRepo(t,
		gitbasetest.Files{".github/promo-flags.json": []byte(`{"freshnessEnforce": false, "docsCoverageEnforce": false, "guardsEnforce": true}`)},
		gitbasetest.Files{"README.md": []byte("hi")})
	tests := []struct {
		name string
		args []string
		code int
		out  string
	}{
		{"no args", nil, 2, ""},
		{"unknown", []string{"nope"}, 2, ""},
		{"flags prints base value", []string{"flags", "-repo", dir}, 0, `"guardsEnforce":true`},
		{"mediaguard walks the diff and succeeds", []string{"mediaguard", "-repo", dir}, 0, ""},
		{"termcheck succeeds on a clean tree", []string{"termcheck", "-repo", dir}, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out, errb bytes.Buffer
			code := run(tt.args, &out, &errb)
			assert.Equal(t, tt.code, code, errb.String())
			assert.True(t, strings.Contains(out.String(), tt.out))
		})
	}
}
