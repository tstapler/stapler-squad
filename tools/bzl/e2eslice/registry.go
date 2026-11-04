package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// RegistryIndex maps a feature id to the source files its registry entry names.
type RegistryIndex map[string][]string

var (
	featureHeader = regexp.MustCompile(`(?m)^\s*//\s*@feature\s+(.+)$`)
	catalogRef    = regexp.MustCompile(`FEATURE_CATALOG\[\s*['"]([^'"]+)['"]\s*\]`)
	catalogEntry  = regexp.MustCompile(`(?s)id:\s*'([^']+)'(.*?)\n\s{2}\},`)
	quoted        = regexp.MustCompile(`'([^']+)'`)
	arrayField    = regexp.MustCompile(`(?s)(componentPaths|rpcIds):\s*\[(.*?)\]`)
)

// specFeatureIDs returns the ids a spec declares: its `// @feature` header plus FEATURE_CATALOG references
// (commented-out references do not count).
func specFeatureIDs(src string) []string {
	var ids []string
	if m := featureHeader.FindStringSubmatch(src); m != nil {
		for _, id := range strings.Split(m[1], ",") {
			if id = strings.TrimSpace(id); id != "" {
				ids = append(ids, id)
			}
		}
	}
	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		for _, m := range catalogRef.FindAllStringSubmatch(line, -1) {
			ids = append(ids, m[1])
		}
	}
	return uniqSorted(ids)
}

// loadRegistry indexes docs/registry/features/**.json and the web feature catalog.
func loadRegistry(repoRoot string) (RegistryIndex, error) {
	idx := RegistryIndex{}
	root := filepath.Join(repoRoot, "docs/registry/features")
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".json") {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		var f struct {
			ID          string `json:"id"`
			HandlerFile string `json:"handlerFile"`
			FilePath    string `json:"filePath"`
		}
		if json.Unmarshal(b, &f) != nil || f.ID == "" {
			return nil
		}
		idx[f.ID] = append(idx[f.ID], nonEmpty(f.HandlerFile, f.FilePath)...)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	cat, _ := filepath.Glob(filepath.Join(repoRoot, "web-app/src/lib/features/features/*.ts"))
	for _, p := range cat {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		for _, m := range catalogEntry.FindAllStringSubmatch(string(b), -1) {
			files := []string{}
			for _, a := range arrayField.FindAllStringSubmatch(m[2], -1) {
				for _, q := range quoted.FindAllStringSubmatch(a[2], -1) {
					if a[1] == "componentPaths" {
						files = append(files, q[1])
					} else if _, ok := idx[q[1]]; !ok {
						idx[q[1]] = nil
					}
				}
			}
			idx[m[1]] = append(idx[m[1]], files...)
		}
	}
	return idx, nil
}

func nonEmpty(xs ...string) []string {
	var out []string
	for _, x := range xs {
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

// Coverage is the ADR-009 registry-coverage table.
type Coverage struct {
	Specs             int                 `json:"specs"`
	WithFeatureID     int                 `json:"with_feature_id"`
	WithResolvableID  int                 `json:"with_resolvable_id"`
	AllIDsResolve     int                 `json:"all_ids_resolve"`
	WithExistingFile  int                 `json:"with_existing_source_file"`
	WithGoFile        int                 `json:"with_go_file"`
	WithWebFile       int                 `json:"with_web_file"`
	DistinctIDs       int                 `json:"distinct_ids"`
	UnresolvedIDs     []string            `json:"unresolved_ids"`
	UnresolvedBySpecs map[string][]string `json:"unresolved_by_spec,omitempty"`
}

// registryCoverage measures how far the hand registry can seed selection.
func registryCoverage(repoRoot string, specs []string, idx RegistryIndex) (Coverage, error) {
	c := Coverage{Specs: len(specs)}
	all, bad := map[string]bool{}, map[string]bool{}
	c.UnresolvedBySpecs = map[string][]string{}
	for _, sp := range specs {
		b, err := os.ReadFile(filepath.Join(repoRoot, sp))
		if err != nil {
			return c, err
		}
		ids := specFeatureIDs(string(b))
		if len(ids) > 0 {
			c.WithFeatureID++
		}
		resolved, every, exist, goF, webF := 0, true, false, false, false
		for _, id := range ids {
			all[id] = true
			files, ok := idx[id]
			if !ok {
				every = false
				bad[id] = true
				c.UnresolvedBySpecs[sp] = append(c.UnresolvedBySpecs[sp], id)
				continue
			}
			resolved++
			for _, f := range files {
				if _, err := os.Stat(filepath.Join(repoRoot, f)); err != nil {
					continue
				}
				exist = true
				goF = goF || strings.HasSuffix(f, ".go")
				webF = webF || strings.HasSuffix(f, ".ts") || strings.HasSuffix(f, ".tsx")
			}
		}
		c.WithResolvableID += b2i(resolved > 0)
		c.AllIDsResolve += b2i(every && len(ids) > 0)
		c.WithExistingFile += b2i(exist)
		c.WithGoFile += b2i(goF)
		c.WithWebFile += b2i(webF)
	}
	c.DistinctIDs = len(all)
	for id := range bad {
		c.UnresolvedIDs = append(c.UnresolvedIDs, id)
	}
	sort.Strings(c.UnresolvedIDs)
	return c, nil
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
