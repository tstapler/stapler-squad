package termscan_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/tools/promolib/termscan"
)

const tsvHeader = "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n"

// tsvRow is one Tesseract word row on line `line` with confidence conf.
func tsvRow(line int, conf float64, text string) string {
	return fmt.Sprintf("5\t1\t1\t1\t%d\t1\t0\t0\t10\t10\t%.2f\t%s\n", line, conf, text)
}

func tsv(rows ...string) []byte {
	return []byte(tsvHeader + "4\t1\t1\t1\t1\t0\t0\t0\t9\t9\t-1\t\n" + strings.Join(rows, ""))
}

func TestParseTSV_KeepsWordRowsOnly(t *testing.T) {
	t.Parallel()
	words, err := termscan.ParseTSV(tsv(tsvRow(1, 91.5, "Ledger"), tsvRow(1, -1, "ghost")))
	require.NoError(t, err)
	require.Len(t, words, 1)
	assert.Equal(t, "Ledger", words[0].Text)
	assert.InDelta(t, 91.5, words[0].Conf, 0.001)
}

func TestParseTSV_ShortRowIsError(t *testing.T) {
	t.Parallel()
	_, err := termscan.ParseTSV([]byte("5\t1\t2\n"))
	require.Error(t, err)
}

func TestScanMediaText_ConfidenceRule(t *testing.T) {
	t.Parallel()
	banned := strings.Join([]string{"war", "hammer"}, "")
	cases := []struct {
		name            string
		conf            float64
		wantFind, wantW int
	}{
		{"confident hit blocks", 95, 1, 0},
		{"at the threshold blocks", 60, 1, 0},
		{"low confidence only warns", 59, 0, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			src := termscan.MemSource{"shot-a.tsv": tsv(tsvRow(1, c.conf, banned))}
			res, err := termscan.ScanMediaText(src, termscan.MediaOptions{MinConfidence: 60})
			require.NoError(t, err)
			assert.Len(t, res.Findings, c.wantFind)
			assert.Len(t, res.Warnings, c.wantW)
			assert.Equal(t, c.wantFind == 0, res.Clean())
		})
	}
}

func TestScanMediaText_DefaultThresholdIs60(t *testing.T) {
	t.Parallel()
	banned := strings.Join([]string{"war", "hammer"}, "")
	src := termscan.MemSource{"a.tsv": tsv(tsvRow(1, 61, banned)), "b.tsv": tsv(tsvRow(1, 59, banned))}
	res, err := termscan.ScanMediaText(src, termscan.MediaOptions{})
	require.NoError(t, err)
	assert.Len(t, res.Findings, 1)
	assert.Len(t, res.Warnings, 1)
}

func TestScanMediaText_NormalisesConfusablesAndNeedsWholeToken(t *testing.T) {
	t.Parallel()
	// 0/O, 1/l and 5/S confusions still match; a longer token that merely contains the term does not.
	short := strings.Join([]string{"40", "k"}, "")
	confused := strings.Join([]string{"4", "O", "K"}, "")
	cases := map[string]bool{
		strings.Join([]string{"WAR", "HAMMER"}, ""):      true,
		strings.Join([]string{"Wa", "rhammer", ","}, ""): true,
		confused:          true,
		"x" + short + "y": false,
		strings.Join([]string{"war", "hammers"}, ""): false,
	}
	for text, want := range cases {
		t.Run(text, func(t *testing.T) {
			t.Parallel()
			res, err := termscan.ScanMediaText(termscan.MemSource{"s.tsv": tsv(tsvRow(1, 99, text))}, termscan.MediaOptions{})
			require.NoError(t, err)
			assert.Equal(t, want, len(res.Findings) == 1, "%v", res.Findings)
		})
	}
}

func TestScanMediaText_MultiWordTermNeedsOneLine(t *testing.T) {
	t.Parallel()
	first, second := strings.Join([]string{"Games"}, ""), strings.Join([]string{"Work", "shop"}, "")
	sameLine := termscan.MemSource{"s.tsv": tsv(tsvRow(1, 90, first), tsvRow(1, 90, second))}
	split := termscan.MemSource{"s.tsv": tsv(tsvRow(1, 90, first), tsvRow(2, 90, second))}
	res, err := termscan.ScanMediaText(sameLine, termscan.MediaOptions{})
	require.NoError(t, err)
	assert.Len(t, res.Findings, 1)
	res, err = termscan.ScanMediaText(split, termscan.MediaOptions{})
	require.NoError(t, err)
	assert.Empty(t, res.Findings)
}

func TestScanMediaText_DOMTextAlwaysBlocks(t *testing.T) {
	t.Parallel()
	src := termscan.MemSource{"dom-a.txt": []byte("see " + strings.Join([]string{"war", "hammer"}, ""))}
	res, err := termscan.ScanMediaText(src, termscan.MediaOptions{MinConfidence: 1})
	require.NoError(t, err)
	require.Len(t, res.Findings, 1)
	assert.Contains(t, res.Findings[0], "dom-a.txt:1")
}

func TestScanMediaText_OCRAllowlist(t *testing.T) {
	t.Parallel()
	banned := strings.Join([]string{"war", "hammer"}, "")
	allow := []termscan.AllowEntry{{Path: "shot-a", Reason: "false positive"}, {Path: "shot-b", Reason: "stale"}, {Path: "other-shard", Reason: "not here"}}
	src := termscan.MemSource{"shot-a.tsv": tsv(tsvRow(1, 99, banned)), "shot-b.tsv": tsv(tsvRow(1, 99, "Ledger"))}
	res, err := termscan.ScanMediaText(src, termscan.MediaOptions{OCRAllow: allow})
	require.NoError(t, err)
	assert.Empty(t, res.Findings, "allow-listed shot does not block")
	assert.Empty(t, res.Warnings)
	require.Len(t, res.Stale, 1, "an entry for a scanned shot with no hit is stale; an absent shot is ignored")
	assert.Equal(t, "shot-b", res.Stale[0].Path)
	assert.False(t, res.Clean())
}
