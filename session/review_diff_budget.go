package session

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	diffManifestReserve = 1500
	diffPerFileCap      = 8000
	diffMinCutBytes     = 200
	diffHeaderPrefix    = "diff --git "
)

// Files that crowd out the item's real change when a byte-prefix cut fills the
// budget (CI, agent settings, lockfiles) are ranked last.
var (
	lowSignalDiffPrefixes = []string{".github/", ".claude/", ".actrc"}
	lowSignalDiffSuffixes = []string{".lock", "go.sum", "package-lock.json", "bun.lockb"}
)

type diffFile struct {
	path  string
	text  string
	noise bool
}

// splitDiffFiles splits at lines starting "diff --git "; any preamble becomes a
// pathless first entry.
func splitDiffFiles(raw string) []diffFile {
	var files []diffFile
	start := 0
	flush := func(end int) {
		if end > start {
			text := raw[start:end]
			path := diffHeaderPath(text)
			files = append(files, diffFile{path: path, text: text, noise: isLowSignalDiffPath(path)})
		}
	}
	for i := 0; i < len(raw); {
		if i > start && strings.HasPrefix(raw[i:], diffHeaderPrefix) {
			flush(i)
			start = i
		}
		nl := strings.IndexByte(raw[i:], '\n')
		if nl < 0 {
			break
		}
		i += nl + 1
	}
	flush(len(raw))
	return files
}

// diffHeaderPath returns the b-side path of the "diff --git" header line,
// handling git's quoted form (`"b/x y"`); "" when it can't be parsed.
func diffHeaderPath(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	if i := strings.LastIndex(line, ` "b/`); i >= 0 {
		if p, err := strconv.Unquote(line[i+1:]); err == nil {
			return strings.TrimPrefix(p, "b/")
		}
	}
	if i := strings.LastIndex(line, " b/"); i >= 0 {
		return line[i+3:]
	}
	return ""
}

func isLowSignalDiffPath(p string) bool {
	for _, pre := range lowSignalDiffPrefixes {
		if strings.HasPrefix(p, pre) {
			return true
		}
	}
	for _, suf := range lowSignalDiffSuffixes {
		if strings.HasSuffix(p, suf) {
			return true
		}
	}
	return false
}

// cutAtLine returns at most n bytes of s ending on a line boundary (falling
// back to a rune boundary when the first line alone is longer than n).
func cutAtLine(s string, n int) string {
	if n >= len(s) {
		return s
	}
	if i := strings.LastIndexByte(s[:n], '\n'); i >= 0 {
		return s[:i+1]
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// TruncateDiffByFile fits raw into limit bytes without letting early or
// low-signal files starve the rest: files are considered real-code-first, each
// kept whole only if it fits under diffPerFileCap, otherwise cut at a line
// boundary. A trailing manifest names cut and omitted files. Output keeps the
// original file order and never exceeds limit. truncated=false means raw fit.
func TruncateDiffByFile(raw string, limit int) (string, bool) {
	if len(raw) <= limit {
		return raw, false
	}
	files := splitDiffFiles(raw)
	if len(files) <= 1 || limit < 2*diffManifestReserve {
		return cutAtLine(raw, limit), true
	}
	order := make([]int, len(files))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return !files[order[a]].noise && files[order[b]].noise })

	kept, cut, dropped := selectDiffFiles(files, order, limit-diffManifestReserve)
	return renderTruncatedDiff(files, kept, cut, dropped), true
}

func selectDiffFiles(files []diffFile, order []int, remaining int) (kept map[int]string, cut, dropped []string) {
	kept = make(map[int]string, len(files))
	var cutIdx []int
	for _, i := range order {
		f := files[i]
		budget := min(remaining, diffPerFileCap)
		switch {
		case len(f.text) <= budget:
			kept[i] = f.text
			remaining -= len(f.text)
		case budget > diffMinCutBytes:
			part := cutAtLine(f.text, budget)
			kept[i] = part
			remaining -= len(part)
			cutIdx = append(cutIdx, i)
		default:
			dropped = append(dropped, f.path)
		}
	}
	// The per-file cap only keeps one file from starving the rest; hand any
	// leftover budget back to the cut files, highest-ranked first.
	for _, i := range cutIdx {
		part := cutAtLine(files[i].text, len(kept[i])+remaining)
		remaining -= len(part) - len(kept[i])
		kept[i] = part
		if len(part) < len(files[i].text) {
			cut = append(cut, files[i].path)
		}
	}
	return kept, cut, dropped
}

func renderTruncatedDiff(files []diffFile, kept map[int]string, cut, dropped []string) string {
	var b strings.Builder
	for i := range files {
		b.WriteString(kept[i])
	}
	manifest := "[diff truncated to fit review budget: " + strconv.Itoa(len(files)-len(dropped)) + " of " + strconv.Itoa(len(files)) + " files shown"
	if len(cut) > 0 {
		manifest += "; cut: " + strings.Join(cut, ", ")
	}
	if len(dropped) > 0 {
		manifest += "; omitted: " + strings.Join(dropped, ", ")
	}
	manifest += "]\n"
	// Reserve leaves room for the manifest plus up to two separator newlines.
	if maxLen := diffManifestReserve - 2; len(manifest) > maxLen {
		manifest = cutAtLine(manifest, maxLen-len("...]\n")) + "...]\n"
	}
	if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
		b.WriteByte('\n')
	}
	fmt.Fprint(&b, "\n"+manifest)
	return b.String()
}
