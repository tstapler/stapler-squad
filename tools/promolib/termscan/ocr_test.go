package termscan_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/tools/promolib/termscan"
)

const requireOCREnv = "TERMSCAN_REQUIRE_OCR"

// TestOCRRecoversKnownText guards the OCR step against a font or Tesseract change:
// the known-text shot must keep yielding the fixed word list, so a regression
// fails in PR CI instead of blocking the weekly deploy. It skips without
// tesseract unless TERMSCAN_REQUIRE_OCR=1.
func TestOCRRecoversKnownText(t *testing.T) {
	t.Parallel()
	bin, err := exec.LookPath("tesseract")
	if err != nil {
		if os.Getenv(requireOCREnv) == "1" {
			t.Fatalf("%s=1 but tesseract is not installed", requireOCREnv)
		}
		t.Skip("tesseract not installed; the OCR fixture is CI-gated")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out := filepath.Join(t.TempDir(), "known-text")
	cmd := safeexec.CommandContext(ctx, bin, filepath.Join("testdata", "ocr", "known-text.png"), out, "tsv")
	require.NoError(t, cmd.Run())
	data, err := os.ReadFile(out + termscan.TSVExt)
	require.NoError(t, err)
	words, err := termscan.ParseTSV(data)
	require.NoError(t, err)

	want := knownWords(t)
	got := map[string]bool{}
	for _, w := range words {
		if w.Conf >= termscan.DefaultOCRMinConfidence {
			got[strings.ToLower(w.Text)] = true
		}
	}
	found := 0
	for _, w := range want {
		if got[strings.ToLower(w)] {
			found++
		}
	}
	recall := float64(found) / float64(len(want))
	assert.GreaterOrEqual(t, recall, fixtureMinRecall(t), "recovered %d of %d known words: %v", found, len(want), got)
}

func knownWords(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "ocr", "known-words.txt"))
	require.NoError(t, err)
	return strings.Fields(string(data))
}

// fixtureMinRecall reads the threshold from tools/promo/budgets.json, its single home.
func fixtureMinRecall(t *testing.T) float64 {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "promo", "budgets.json"))
	require.NoError(t, err)
	var b struct {
		Recall float64 `json:"ocrFixtureMinRecall"`
	}
	require.NoError(t, json.Unmarshal(data, &b))
	require.Greater(t, b.Recall, 0.0, "budgets.json needs ocrFixtureMinRecall")
	return b.Recall
}

// TestKnownTextFixtureIsCommitted keeps the fixture and its word list in step
// even where tesseract is absent.
func TestKnownTextFixtureIsCommitted(t *testing.T) {
	t.Parallel()
	_, err := os.Stat(filepath.Join("testdata", "ocr", "known-text.png"))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(knownWords(t)), 5)
}
