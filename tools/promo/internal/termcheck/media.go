package termcheck

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/tstapler/stapler-squad/tools/promo/internal/manifest"
	"github.com/tstapler/stapler-squad/tools/promolib/termscan"
)

// runMediaText scans DOM text dumps (always blocking) and Tesseract TSV output
// (blocking only at or above the configured word confidence; lower hits warn).
func runMediaText(dir, budgetsPath, ocrAllowPath string, stdout, stderr io.Writer) int {
	if dir == "" {
		fmt.Fprintln(stderr, "termcheck: -dir is required for -mode=media-text")
		return ExitError
	}
	opts, err := mediaOptions(budgetsPath, ocrAllowPath)
	if err != nil {
		fmt.Fprintln(stderr, "termcheck:", err)
		return ExitError
	}
	res, err := termscan.ScanMediaText(termscan.DirSource{Dir: dir}, opts)
	if err != nil {
		fmt.Fprintln(stderr, "termcheck:", err)
		return ExitError
	}
	for _, f := range res.Findings {
		fmt.Fprintln(stdout, f)
	}
	for _, s := range res.Stale {
		fmt.Fprintf(stdout, "stale OCR allow-list entry (shot has no banned term): %s\n", s)
	}
	reportWarnings(res.Warnings, stdout)
	if res.Clean() {
		return ExitOK
	}
	return ExitFindings
}

func mediaOptions(budgetsPath, ocrAllowPath string) (termscan.MediaOptions, error) {
	var opts termscan.MediaOptions
	switch b, err := manifest.ReadBudgets(budgetsPath); {
	case err == nil:
		opts.MinConfidence = b.OCRMinConfidence
	case errors.Is(err, os.ErrNotExist):
	default:
		return opts, err
	}
	data, err := os.ReadFile(ocrAllowPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return opts, nil
		}
		return opts, err
	}
	opts.OCRAllow, err = termscan.ParseAllowlist(data)
	return opts, err
}

// reportWarnings prints low-confidence OCR hits and appends them to the job
// summary when running in GitHub Actions. They never change the exit code.
func reportWarnings(warnings []string, stdout io.Writer) {
	if len(warnings) == 0 {
		return
	}
	for _, w := range warnings {
		fmt.Fprintln(stdout, "warning:", w)
	}
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, "### Low-confidence OCR hits (not blocking)")
	for _, w := range warnings {
		fmt.Fprintf(f, "- %s\n", w)
	}
}
