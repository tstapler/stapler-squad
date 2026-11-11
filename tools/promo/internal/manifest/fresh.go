package manifest

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Problem is one freshness failure, printed as "<subject>: <message>".
type Problem struct{ Subject, Message string }

func (p Problem) String() string { return p.Subject + ": " + p.Message }

// FreshInput is everything the freshness check needs. It performs no network
// access: the compare result is supplied by the workflow.
type FreshInput struct {
	Manifest MediaManifest
	Expected ExpectedMedia
	Budgets  Budgets
	// DistDir enables the dist existence and total-size checks when non-empty.
	DistDir string
	SHA     string
	// ReuseFrom switches the commit check to reuse mode (Compare is then required).
	ReuseFrom string
	Compare   *Compare
}

// Fresh returns every freshness problem; empty means fresh.
func Fresh(in FreshInput) []Problem {
	var out []Problem
	out = append(out, checkPresence(in.Manifest, in.Expected)...)
	out = append(out, checkCommit(in)...)
	for _, e := range in.Manifest.Entries {
		if e.Status != StatusRecorded {
			continue
		}
		if e.ThemeID != ThemeID {
			out = append(out, Problem{e.Name, fmt.Sprintf("themeId %q, want %q", e.ThemeID, ThemeID)})
		}
		out = append(out, checkBudgets(e, in.Budgets)...)
		out = append(out, checkDist(e, in.DistDir)...)
	}
	out = append(out, checkWallClock(in.Manifest, in.Budgets)...)
	out = append(out, checkDistTotal(in.DistDir, in.Budgets)...)
	return out
}

func checkPresence(m MediaManifest, exp ExpectedMedia) []Problem {
	var out []Problem
	for _, item := range exp.Media {
		formats := item.Formats
		if len(formats) == 0 {
			if !hasEntry(m, item.Name, "") {
				out = append(out, Problem{item.Name, missingOrFailed(m, item.Name, "")})
			}
			continue
		}
		for _, f := range formats {
			if !hasEntry(m, item.Name, f) {
				out = append(out, Problem{fmt.Sprintf("%s (%s)", item.Name, f), missingOrFailed(m, item.Name, f)})
			}
		}
	}
	return out
}

func hasEntry(m MediaManifest, name string, f Format) bool {
	for _, e := range m.Entries {
		if e.Name == name && (f == "" || e.Format == f) && e.Status == StatusRecorded {
			return true
		}
	}
	return false
}

func missingOrFailed(m MediaManifest, name string, f Format) string {
	for _, e := range m.Entries {
		if e.Name == name && (f == "" || e.Format == f) {
			return string(e.Status)
		}
	}
	return "missing"
}

func checkCommit(in FreshInput) []Problem {
	var out []Problem
	want := in.SHA
	if in.ReuseFrom != "" {
		want = in.ReuseFrom
		if in.Compare == nil {
			return []Problem{{"reuse", "-reuse-from needs the compare result (-compare-json)"}}
		}
		if d := DecideReuse(*in.Compare, in.Budgets.RecordInputPaths); !d.Reusable {
			out = append(out, Problem{"reuse", d.Reason})
		}
	}
	for _, e := range in.Manifest.Entries {
		if e.Status == StatusRecorded && e.Commit != want {
			out = append(out, Problem{e.Name, fmt.Sprintf("commit %s, want %s", e.Commit, want)})
		}
	}
	return out
}

func checkBudgets(e MediaEntry, b Budgets) []Problem {
	var out []Problem
	if e.Format.IsClip() {
		if e.Bytes > b.ClipMaxBytes {
			out = append(out, Problem{e.Name, fmt.Sprintf("clipMaxBytes exceeded: %d > %d", e.Bytes, b.ClipMaxBytes)})
		}
		if e.DurationMs > b.ClipMaxDurationMs {
			out = append(out, Problem{e.Name, fmt.Sprintf("clipMaxDurationMs exceeded: %d > %d", e.DurationMs, b.ClipMaxDurationMs)})
		}
	} else if e.Bytes > b.ImageMaxBytes {
		out = append(out, Problem{e.Name, fmt.Sprintf("imageMaxBytes exceeded: %d > %d", e.Bytes, b.ImageMaxBytes)})
	}
	return out
}

func checkDist(e MediaEntry, distDir string) []Problem {
	if distDir == "" {
		return nil
	}
	var out []Problem
	refs := []struct{ field, ref string }{{"src", e.Src}, {"poster", e.Poster}, {"source", e.Source}}
	for _, r := range refs {
		field, ref := r.field, r.ref
		if ref == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(distDir, filepath.FromSlash(strings.TrimPrefix(ref, "/")))); err != nil {
			out = append(out, Problem{e.Name, fmt.Sprintf("%s %q missing from dist", field, ref)})
		}
	}
	return out
}

func checkWallClock(m MediaManifest, b Budgets) []Problem {
	if m.StartedAt.IsZero() || m.FinishedAt.IsZero() {
		return nil
	}
	limit := time.Duration(b.RecordWallClockMaxMinutes) * time.Minute
	if d := m.FinishedAt.Sub(m.StartedAt); d > limit {
		return []Problem{{"record", fmt.Sprintf("recordWallClockMaxMinutes exceeded: %s > %s", d, limit)}}
	}
	return nil
}

// DistSize totals the file sizes under dir.
func DistSize(dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	return total, err
}

func checkDistTotal(distDir string, b Budgets) []Problem {
	if distDir == "" {
		return nil
	}
	total, err := DistSize(distDir)
	if err != nil {
		return []Problem{{"dist", err.Error()}}
	}
	if total > b.DistMaxBytes {
		return []Problem{{"dist", fmt.Sprintf("distMaxBytes exceeded: %d > %d", total, b.DistMaxBytes)}}
	}
	return nil
}
