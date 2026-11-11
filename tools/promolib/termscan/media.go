package termscan

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// TSVExt marks Tesseract TSV output (`tesseract in out tsv`) in a media-text
// directory; every other file there is plain DOM text and always blocks.
const TSVExt = ".tsv"

// DefaultOCRMinConfidence is used when MediaOptions.MinConfidence is zero.
const DefaultOCRMinConfidence = 60.0

// OCRWord is one recognised word with Tesseract's confidence (0-100).
type OCRWord struct {
	Text string
	Conf float64
	// Line groups words of the same OCR text line (block, paragraph, line).
	Line string
}

// ParseTSV reads Tesseract TSV output and returns its word rows (level 5).
// Rows with negative confidence or empty text are layout rows and skipped.
func ParseTSV(data []byte) ([]OCRWord, error) {
	var out []OCRWord
	for i, row := range strings.Split(string(data), "\n") {
		row = strings.TrimRight(row, "\r")
		if row == "" || i == 0 && strings.HasPrefix(row, "level\t") {
			continue
		}
		cols := strings.Split(row, "\t")
		if len(cols) < 12 {
			return nil, fmt.Errorf("tsv row %d: want 12 columns, got %d", i+1, len(cols))
		}
		if cols[0] != "5" {
			continue
		}
		conf, err := strconv.ParseFloat(cols[10], 64)
		if err != nil {
			return nil, fmt.Errorf("tsv row %d: confidence %q: %w", i+1, cols[10], err)
		}
		text := strings.TrimSpace(cols[11])
		if conf < 0 || text == "" {
			continue
		}
		out = append(out, OCRWord{Text: text, Conf: conf, Line: strings.Join(cols[2:5], ".")})
	}
	return out, nil
}

// foldOCR lowercases and folds the characters Tesseract confuses (0/O, 1/l/I,
// 5/S) onto one form, so "WARHAMMER" misread as "WARHAMMER" or "wh4Ok" compare equal.
func foldOCR(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case 'o', 'O':
			return '0'
		case 'l', 'L', 'i', 'I', '|':
			return '1'
		case 's', 'S':
			return '5'
		}
		return unicode.ToLower(r)
	}, s)
}

func termParts(t Term) []string {
	return strings.FieldsFunc(t.Text, func(r rune) bool { return r == ' ' || r == '-' || r == '_' })
}

// OCRHit is a banned term found in OCR output.
type OCRHit struct {
	Shot string
	Term string
	// MinConf is the lowest word confidence across the matched tokens.
	MinConf float64
}

func (h OCRHit) String() string {
	return fmt.Sprintf("%s: banned term %q in OCR text (confidence %.0f)", h.Shot, h.Term, h.MinConf)
}

// FindOCRHits matches banned terms against whole OCR tokens, a multi-word term
// against consecutive tokens of one OCR line.
func FindOCRHits(shot string, words []OCRWord) []OCRHit {
	var hits []OCRHit
	for _, t := range BannedTermSet() {
		parts := termParts(t)
		if len(parts) == 0 {
			continue
		}
		folded := make([]string, len(parts))
		for i, p := range parts {
			folded[i] = foldOCR(p)
		}
		for i := 0; i+len(parts) <= len(words); i++ {
			if h, ok := matchAt(shot, t, folded, words[i:i+len(parts)]); ok {
				hits = append(hits, h)
			}
		}
	}
	return hits
}

func matchAt(shot string, t Term, folded []string, window []OCRWord) (OCRHit, bool) {
	min := window[0].Conf
	for i, w := range window {
		if w.Line != window[0].Line || foldOCR(strings.Trim(w.Text, ".,:;!?\"'()[]")) != folded[i] {
			return OCRHit{}, false
		}
		if w.Conf < min {
			min = w.Conf
		}
	}
	return OCRHit{Shot: shot, Term: t.Text, MinConf: min}, true
}

// MediaOptions tune ScanMediaText.
type MediaOptions struct {
	// MinConfidence is the Tesseract word confidence a hit needs to block
	// (zero means DefaultOCRMinConfidence).
	MinConfidence float64
	// OCRAllow lists shots with confirmed OCR false positives (Path is the shot name).
	OCRAllow []AllowEntry
}

// MediaResult is the outcome of a media-text scan.
type MediaResult struct {
	// Findings block: any hit in DOM text, and OCR hits at or above MinConfidence.
	Findings []string
	// Warnings never block: OCR hits below MinConfidence.
	Warnings []string
	// Stale lists OCR allow-list entries whose shot was scanned but produced no hit.
	Stale []AllowEntry
}

// Clean reports whether nothing should fail the job.
func (r MediaResult) Clean() bool { return len(r.Findings) == 0 && len(r.Stale) == 0 }

type partitionSource struct {
	Source
	keep func(string) bool
}

func (p partitionSource) Files() ([]string, error) {
	all, err := p.Source.Files()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, f := range all {
		if p.keep(f) {
			out = append(out, f)
		}
	}
	return out, nil
}

func isTSV(p string) bool { return strings.HasSuffix(p, TSVExt) }

// ScanMediaText scans a media-text directory: plain files (DOM text dumps) block
// on any banned term; *.tsv files (OCR output) block only when the hit's word
// confidence reaches opts.MinConfidence and otherwise warn. An allow-listed shot
// never blocks or warns; an entry for a scanned shot with no hit is stale. An
// entry for a shot absent from src is ignored (a shard holds only its own shots).
func ScanMediaText(src Source, opts MediaOptions) (MediaResult, error) {
	minConf := opts.MinConfidence
	if minConf == 0 {
		minConf = DefaultOCRMinConfidence
	}
	var res MediaResult
	dom, err := Scan(partitionSource{src, func(p string) bool { return !isTSV(p) }}, Options{AllPaths: true})
	if err != nil {
		return res, err
	}
	for _, f := range dom.Findings {
		res.Findings = append(res.Findings, f.String())
	}
	if err := scanOCR(src, minConf, opts.OCRAllow, &res); err != nil {
		return res, err
	}
	return res, nil
}

// scanOCR classifies every *.tsv shot in src into res.
func scanOCR(src Source, minConf float64, allow []AllowEntry, res *MediaResult) error {
	files, err := src.Files()
	if err != nil {
		return err
	}
	allowed := make(map[string]bool, len(allow))
	for _, a := range allow {
		allowed[a.Path] = true
	}
	hitShots := map[string]bool{}
	scanned := map[string]bool{}
	sort.Strings(files)
	for _, f := range files {
		if !isTSV(f) {
			continue
		}
		shot := strings.TrimSuffix(path.Base(f), TSVExt)
		scanned[shot] = true
		data, err := src.Read(f)
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}
		words, err := ParseTSV(data)
		if err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
		for _, h := range FindOCRHits(shot, words) {
			hitShots[shot] = true
			switch {
			case allowed[shot]:
			case h.MinConf >= minConf:
				res.Findings = append(res.Findings, h.String())
			default:
				res.Warnings = append(res.Warnings, h.String()+fmt.Sprintf(" is below the minimum %.0f; not blocking", minConf))
			}
		}
	}
	for _, a := range allow {
		if scanned[a.Path] && !hitShots[a.Path] {
			res.Stale = append(res.Stale, a)
		}
	}
	return nil
}
