package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry is one spec's observed dependencies (ADR-009 observed-dependency manifest).
type Entry struct {
	Spec           string   `json:"spec"`
	GoPkgs         []string `json:"go_pkgs"`
	WebFiles       []string `json:"web_files"`
	Routes         []string `json:"routes"`
	RPCs           []string `json:"rpcs"`
	NoServer       bool     `json:"no_server,omitempty"`
	NoServerReason string   `json:"no_server_reason,omitempty"`
	Excluded       string   `json:"excluded,omitempty"` // reason the spec cannot be captured; it always runs
}

// Manifest is tests/e2e/slice-manifest.json.
type Manifest struct {
	GeneratedAt string  `json:"generated_at"`
	Specs       []Entry `json:"specs"`
}

func readManifest(path string) (*Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &m, nil
}

func (m *Manifest) normalize() {
	for i := range m.Specs {
		e := &m.Specs[i]
		e.GoPkgs, e.WebFiles, e.Routes, e.RPCs = uniqSorted(e.GoPkgs), uniqSorted(e.WebFiles), uniqSorted(e.Routes), uniqSorted(e.RPCs)
	}
	sort.Slice(m.Specs, func(i, j int) bool { return m.Specs[i].Spec < m.Specs[j].Spec })
}

func (m *Manifest) write(path string) error {
	m.normalize()
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func uniqSorted(in []string) []string {
	set := map[string]bool{}
	for _, s := range in {
		set[s] = true
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// listSpecs returns the top-level specs the CI shards run: <e2eDir>/*.spec.ts, as repo-relative slash paths.
func listSpecs(repoRoot, e2eDir string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(repoRoot, e2eDir, "*.spec.ts"))
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(files))
	for _, f := range files {
		rel, err := filepath.Rel(repoRoot, f)
		if err != nil {
			return nil, err
		}
		out = append(out, filepath.ToSlash(rel))
	}
	sort.Strings(out)
	return out, nil
}

// checkManifest implements the Story 12.1 AC: every spec exactly once, with deps or a recorded reason.
// It returns one problem per violation; an empty slice means the manifest is valid.
func checkManifest(m *Manifest, specsOnDisk []string) []string {
	var problems []string
	if m.GeneratedAt == "" {
		problems = append(problems, "generated_at (commit) is empty")
	}
	seen := map[string]int{}
	for i, e := range m.Specs {
		seen[e.Spec]++
		if i > 0 && m.Specs[i-1].Spec >= e.Spec && seen[e.Spec] == 1 {
			problems = append(problems, "specs are not sorted at "+e.Spec)
		}
		switch {
		case e.Excluded != "":
		case e.NoServer && e.NoServerReason == "":
			problems = append(problems, e.Spec+": no_server without no_server_reason")
		case !e.NoServer && len(e.GoPkgs) == 0:
			problems = append(problems, e.Spec+": empty go_pkgs (and not no_server or excluded)")
		}
	}
	for s, n := range seen {
		if n > 1 {
			problems = append(problems, fmt.Sprintf("%s: appears %d times", s, n))
		}
	}
	for _, s := range specsOnDisk {
		if seen[s] == 0 {
			problems = append(problems, s+": unmanaged (no manifest entry)")
		}
	}
	onDisk := map[string]bool{}
	for _, s := range specsOnDisk {
		onDisk[s] = true
	}
	for s := range seen {
		if !onDisk[s] && strings.HasSuffix(s, ".spec.ts") {
			problems = append(problems, s+": manifest entry for a spec that no longer exists")
		}
	}
	sort.Strings(problems)
	return problems
}

// merge unions captures (several runs of the same spec) into a manifest entry per spec.
func merge(commit string, captures []Entry) *Manifest {
	by := map[string]*Entry{}
	for _, c := range captures {
		e, ok := by[c.Spec]
		if !ok {
			cp := c
			by[c.Spec] = &cp
			continue
		}
		e.GoPkgs = append(e.GoPkgs, c.GoPkgs...)
		e.WebFiles = append(e.WebFiles, c.WebFiles...)
		e.Routes = append(e.Routes, c.Routes...)
		e.RPCs = append(e.RPCs, c.RPCs...)
		e.NoServer = e.NoServer && c.NoServer
		if e.Excluded == "" {
			e.Excluded = c.Excluded
		}
	}
	m := &Manifest{GeneratedAt: commit}
	for _, e := range by {
		m.Specs = append(m.Specs, *e)
	}
	m.normalize()
	return m
}
