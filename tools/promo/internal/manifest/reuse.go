package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/bmatcuk/doublestar/v4"
)

// MaxCompareFiles is the compare API's file ceiling; a list this long may be truncated.
const MaxCompareFiles = 3000

// CompareFile is one file of the compare API response.
type CompareFile struct {
	Filename         string `json:"filename"`
	PreviousFilename string `json:"previous_filename"`
}

// Compare is the part of `gh api repos/{owner}/{repo}/compare/<a>...<b> --paginate`
// the reuse decision needs.
type Compare struct {
	Status    string        `json:"status"`
	Files     []CompareFile `json:"files"`
	Truncated bool          `json:"truncated"`
}

// ParseCompare reads one or more concatenated compare pages (--paginate output),
// taking the status from the first page and every file from all pages.
func ParseCompare(r io.Reader) (Compare, error) {
	dec := json.NewDecoder(r)
	var out Compare
	for first := true; ; first = false {
		var page Compare
		if err := dec.Decode(&page); errors.Is(err, io.EOF) {
			if first {
				return Compare{}, errors.New("parse compare JSON: empty input")
			}
			break
		} else if err != nil {
			return Compare{}, fmt.Errorf("parse compare JSON: %w", err)
		}
		if first {
			out.Status = page.Status
		}
		out.Files = append(out.Files, page.Files...)
		out.Truncated = out.Truncated || page.Truncated
	}
	if len(out.Files) >= MaxCompareFiles {
		out.Truncated = true
	}
	return out, nil
}

// ReuseDecision says whether media recorded at a past commit can be reused.
type ReuseDecision struct {
	Reusable bool
	Reason   string
}

// DecideReuse allows reuse only when recorded is an ancestor of (or equal to) the
// current commit and no recording-input path changed between them. Renames count
// on both the old and the new name.
func DecideReuse(cmp Compare, inputPaths []string) ReuseDecision {
	if cmp.Status != "ahead" && cmp.Status != "identical" {
		return ReuseDecision{Reason: fmt.Sprintf("compare status %q: recorded commit is not an ancestor", cmp.Status)}
	}
	if cmp.Truncated {
		return ReuseDecision{Reason: "compare file list is truncated"}
	}
	for _, f := range cmp.Files {
		for _, name := range []string{f.Filename, f.PreviousFilename} {
			if name != "" && matchesAny(inputPaths, name) {
				return ReuseDecision{Reason: "recording input changed: " + name}
			}
		}
	}
	return ReuseDecision{Reusable: true}
}

func matchesAny(globs []string, p string) bool {
	for _, g := range globs {
		if ok, _ := doublestar.Match(g, p); ok {
			return true
		}
	}
	return false
}
