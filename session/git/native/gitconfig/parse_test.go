package gitconfig

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// listing renders entries the way `git config --list -z` prints them, one string per entry.
func listing(entries []Entry) []string {
	var out []string
	for _, e := range entries {
		if e.HasValue {
			out = append(out, e.Name()+"="+e.Value)
		} else {
			out = append(out, e.Name())
		}
	}
	return out
}

func TestParseContract(t *testing.T) {
	for _, c := range tokenizerCases() {
		t.Run(c.name, func(t *testing.T) {
			got, err := Parse([]byte(c.content))
			if c.wantErr {
				var ce *Error
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, SubParse, ce.Sub)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.want, listing(got))
		})
	}
}

func TestParseKeepsStructure(t *testing.T) {
	got, err := Parse([]byte("[Remote \"Origin\"]\n\tURL = u\n[a]\nbare\n"))
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, Entry{Section: "remote", Subsection: "Origin", HasSubsection: true, Key: "url", Value: "u", HasValue: true, Line: 2}, got[0])
	assert.Equal(t, Entry{Section: "a", Key: "bare", Line: 4}, got[1])
}

func TestParseNeverPanicsOnGarbage(t *testing.T) {
	for _, s := range []string{"[", "[a", "[a \"", "[a \"x\\", "k =", "k = \\", "\\", "\"", "[\"]", "\x00\x01", strings.Repeat("[", 100)} {
		_, _ = Parse([]byte(s))
	}
}

func FuzzParse(f *testing.F) {
	for _, c := range tokenizerCases() {
		f.Add([]byte(c.content))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		entries, err := Parse(data)
		if err == nil {
			for _, e := range entries {
				if e.Section != "" && e.Key == "" {
					t.Fatalf("entry without a key: %+v", e)
				}
			}
		}
	})
}
