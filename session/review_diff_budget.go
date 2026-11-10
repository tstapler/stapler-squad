package session

import (
	"fmt"
	"sort"
	"strings"
)

const (
	diffManifestReserve = 1500
	diffPerFileCap      = 8000
)

// lowSignalDiffPrefixes/Suffixes mark files that crowd out the item's real
// change when a byte-prefix cut fills the budget (CI, agent settings, lockfiles).
var (
	lowSignalDiffPrefixes = []string{".github/", ".claude/", ".actrc"}
	lowSignalDiffSuffixes = []string{".lock", "go.sum", "package-lock.json", "bun.lock", "bun.lockb"}
)

type diffFile struct {
	idx   int
	path  string
	text  string
	noise bool
}

func splitDiffFiles(raw string) []diffFile {
	var files []diffFile
	start := 0
	flush := func(end int) {
		if end <= start {
			return
		}
		text := raw[start:end]
		path := ""
		if nl := strings.IndexByte(text, '\n'); nl > 0 {
			if i := strings.LastIndex(text[:nl], " b/"); i >= 0 {
				path = text[i+3 : nl]
			}
		}
		files = append(files, diffFile{idx: len(files), path: path, text: text, noise: isLowSignalDiffPath(path)})
	}
	for i := 0; i < len(raw); {
		if strings.HasPrefix(raw[i:], "diff --git ") && i > start {
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

// TruncateDiffByFile fits raw into max bytes without letting early or
// low-signal files starve the rest: every file is considered in priority
// order (real code first), each capped at diffPerFileCap, and a manifest names
// files that were cut or dropped so the reviewer knows what it did not see.
// Output keeps the original file order. Returns truncated=false when raw fits.
func TruncateDiffByFile(raw string, max int) (string, bool) {
	if len(raw) <= max {
		return raw, false
	}
	files := splitDiffFiles(raw)
	if len(files) <= 1 {
		return raw[:max], true
	}
	order := make([]int, len(files))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return !files[order[a]].noise && files[order[b]].noise })

	kept, cut, dropped := selectDiffFiles(files, order, max-diffManifestReserve, max/2)
	return renderTruncatedDiff(files, kept, cut, dropped), true
}

func selectDiffFiles(files []diffFile, order []int, remaining, wholeFileMax int) (kept map[int]string, cut, dropped []string) {
	kept = make(map[int]string, len(files))
	for _, i := range order {
		f := files[i]
		budget := min(remaining, diffPerFileCap)
		switch {
		case len(f.text) <= remaining && len(f.text) <= wholeFileMax:
			kept[i] = f.text
			remaining -= len(f.text)
		case budget > 200:
			kept[i] = f.text[:budget]
			remaining -= budget
			cut = append(cut, f.path)
		default:
			dropped = append(dropped, f.path)
		}
	}
	return kept, cut, dropped
}

func renderTruncatedDiff(files []diffFile, kept map[int]string, cut, dropped []string) string {
	var b strings.Builder
	for i := range files {
		if t, ok := kept[i]; ok {
			b.WriteString(t)
			if !strings.HasSuffix(t, "\n") {
				b.WriteByte('\n')
			}
		}
	}
	manifest := fmt.Sprintf("\n[diff truncated to fit review budget: %d of %d files shown in full or in part; cut: %s; omitted: %s]\n",
		len(files)-len(dropped), len(files), strings.Join(cut, ", "), strings.Join(dropped, ", "))
	if len(manifest) > diffManifestReserve {
		manifest = manifest[:diffManifestReserve-1] + "]\n"
	}
	b.WriteString(manifest)
	return b.String()
}
