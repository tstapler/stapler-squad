package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Line is an inclusive 1-based source line range.
type Line [2]int

// parseTextfmt reads `go tool covdata textfmt` output ("pkg/file.go:12.3,15.4 stmts count") and returns, per
// repo-relative file, the merged line ranges whose count is positive. Files with only zero-count blocks are absent.
func parseTextfmt(text string) map[string][]Line {
	hit, _ := parseTextfmtAll(text)
	return hit
}

// parseTextfmtAll also returns every instrumented statement block (executed or not), merged per file, so a
// selector can tell "edited a covered-but-unexecuted line" from "edited a line outside any function body".
func parseTextfmtAll(text string) (executed, all map[string][]Line) {
	raw := map[string][]Line{}
	every := map[string][]Line{}
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(l, "mode:") || strings.TrimSpace(l) == "" {
			continue
		}
		colon := strings.LastIndex(l, ":")
		if colon < 0 || !strings.HasPrefix(l, modulePath+"/") {
			continue
		}
		file := strings.TrimPrefix(l[:colon], modulePath+"/")
		f := strings.Fields(l[colon+1:])
		if len(f) != 3 {
			continue
		}
		n, nerr := strconv.Atoi(f[2])
		span := strings.SplitN(f[0], ",", 2)
		if len(span) != 2 {
			continue
		}
		start, e1 := strconv.Atoi(strings.SplitN(span[0], ".", 2)[0])
		end, e2 := strconv.Atoi(strings.SplitN(span[1], ".", 2)[0])
		if e1 != nil || e2 != nil {
			continue
		}
		every[file] = append(every[file], Line{start, end})
		if nerr == nil && n > 0 {
			raw[file] = append(raw[file], Line{start, end})
		}
	}
	for f, ls := range raw {
		raw[f] = mergeLines(ls)
	}
	for f, ls := range every {
		every[f] = mergeLines(ls)
	}
	return raw, every
}

func mergeLines(ls []Line) []Line {
	sort.Slice(ls, func(i, j int) bool { return ls[i][0] < ls[j][0] })
	var out []Line
	for _, l := range ls {
		if n := len(out); n > 0 && l[0] <= out[n-1][1]+1 {
			if l[1] > out[n-1][1] {
				out[n-1][1] = l[1]
			}
			continue
		}
		out = append(out, l)
	}
	return out
}

// v8Func mirrors the Chromium coverage function entries the preload records.
type v8Func struct {
	Ranges []struct {
		Start int `json:"startOffset"`
		End   int `json:"endOffset"`
		Count int `json:"count"`
	} `json:"ranges"`
}

type webCovFile struct {
	Entries []struct {
		URL       string   `json:"url"`
		Functions []v8Func `json:"functions"`
	} `json:"entries"`
	Paths []string `json:"paths"`
}

// coveredOffsets paints V8 block ranges (outer first, so inner ranges override) and returns a per-offset executed flag.
func coveredOffsets(funcs []v8Func, size int) []bool {
	type rg struct{ s, e, c int }
	var all []rg
	for _, f := range funcs {
		for _, r := range f.Ranges {
			all = append(all, rg{r.Start, r.End, r.Count})
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].s != all[j].s {
			return all[i].s < all[j].s
		}
		return all[i].e > all[j].e
	})
	cov := make([]bool, size+1)
	for _, r := range all {
		for o := r.s; o < r.e && o <= size; o++ {
			cov[o] = r.c > 0
		}
	}
	return cov
}

// sourceMap is the part of a v3 map this tool needs.
type sourceMap struct {
	Sources  []string `json:"sources"`
	Mappings string   `json:"mappings"`
}

const b64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

func vlq(s string, i *int) int {
	result, shift := 0, 0
	for *i < len(s) {
		d := strings.IndexByte(b64, s[*i])
		*i++
		result += (d & 31) << shift
		if d&32 == 0 {
			break
		}
		shift += 5
	}
	if result&1 == 1 {
		return -(result >> 1)
	}
	return result >> 1
}

// segment is one decoded mapping: generated (line, col), source index (-1 when the segment has none).
type segment struct{ line, col, src int }

func decodeMappings(m string) []segment {
	var segs []segment
	line, src := 0, 0
	for _, ln := range strings.Split(m, ";") {
		col := 0
		for _, part := range strings.Split(ln, ",") {
			if part == "" {
				continue
			}
			i := 0
			col += vlq(part, &i)
			s := -1
			if i < len(part) {
				src += vlq(part, &i)
				s = src
				vlq(part, &i) // source line
				vlq(part, &i) // source column
			}
			segs = append(segs, segment{line, col, s})
		}
		line++
	}
	return segs
}

// repoSource turns a map source ("../src/lib/x.tsx") into a repo path under web-app, or "" for dependencies.
func repoSource(s string) string {
	if strings.Contains(s, "node_modules") {
		return ""
	}
	if i := strings.Index(s, "src/"); i >= 0 {
		return path.Join("web-app", s[i:])
	}
	return ""
}

// webSources resolves the covered generated code of one chunk to the repo files whose mapped segments were executed.
func webSources(js []byte, mapJSON []byte, cov []bool) ([]string, error) {
	var sm sourceMap
	if err := json.Unmarshal(mapJSON, &sm); err != nil {
		return nil, err
	}
	lineStart := []int{0}
	for i, b := range js {
		if b == '\n' {
			lineStart = append(lineStart, i+1)
		}
	}
	set := map[string]bool{}
	for _, sg := range decodeMappings(sm.Mappings) {
		if sg.src < 0 || sg.src >= len(sm.Sources) || sg.line >= len(lineStart) {
			continue
		}
		off := lineStart[sg.line] + sg.col
		if off < len(cov) && cov[off] {
			if p := repoSource(sm.Sources[sg.src]); p != "" {
				set[p] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

// webCapture reads every preload file under webDir and resolves chunk coverage through the dist source maps.
func webCapture(webDir, distDir string) (files, paths []string, err error) {
	jsons, _ := filepath.Glob(filepath.Join(webDir, "*.json"))
	fset, pset := map[string]bool{}, map[string]bool{}
	for _, jf := range jsons {
		b, rerr := os.ReadFile(jf)
		if rerr != nil {
			return nil, nil, rerr
		}
		var w webCovFile
		if jerr := json.Unmarshal(b, &w); jerr != nil {
			return nil, nil, fmt.Errorf("%s: %w", jf, jerr)
		}
		for _, p := range w.Paths {
			pset[p] = true
		}
		for _, e := range w.Entries {
			u := e.URL
			if i := strings.Index(u, "/_next/"); i >= 0 {
				u = u[i:]
			} else {
				continue
			}
			js, jerr := os.ReadFile(filepath.Join(distDir, filepath.FromSlash(u)))
			if jerr != nil {
				continue
			}
			mp, merr := os.ReadFile(filepath.Join(distDir, filepath.FromSlash(u)+".map"))
			if merr != nil {
				continue
			}
			srcs, serr := webSources(js, mp, coveredOffsets(e.Functions, len(js)))
			if serr != nil {
				return nil, nil, serr
			}
			for _, s := range srcs {
				fset[s] = true
			}
		}
	}
	for s := range fset {
		files = append(files, s)
	}
	for p := range pset {
		paths = append(paths, p)
	}
	sort.Strings(files)
	sort.Strings(paths)
	return files, paths, nil
}

// rpcsOf picks Connect RPC method paths ("/pkg.Service/Method") out of the recorded request paths.
func rpcsOf(paths []string) []string {
	var out []string
	for _, p := range paths {
		p = strings.TrimPrefix(p, "/api")
		parts := strings.Split(strings.Trim(p, "/"), "/")
		if len(parts) == 2 && strings.Contains(parts[0], ".") && !strings.Contains(parts[1], ".") {
			out = append(out, "/"+parts[0]+"/"+parts[1])
		}
	}
	sort.Strings(out)
	return out
}

// withFileDirs adds the package of every executed file: `covdata percent` rounds a package with a few executed
// statements to 0.0%, so executedPackages alone drops small packages that did run.
func withFileDirs(pkgs, files []string) []string {
	set := map[string]bool{}
	for _, p := range pkgs {
		set[p] = true
	}
	for _, f := range files {
		if d := path.Dir(f); d == "." {
			set[modulePath] = true
		} else {
			set[modulePath+"/"+d] = true
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
