// Package registry loads the feature registry and the docs-coverage sidecars.
package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/model"
	"github.com/tstapler/stapler-squad/tools/promolib/gitbase"
)

// Repo-relative locations.
const (
	FeaturesDir = "docs/registry/features"
	SidecarDir  = "docs/registry/docs-coverage"
)

// Files maps a repo-relative path to its content.
type Files map[string][]byte

// Registry is every feature keyed by id (not filename).
type Registry struct {
	Features map[model.FeatureID]model.Feature
	Warnings []string
}

// IDs returns the ids in sorted order.
func (r Registry) IDs() []model.FeatureID {
	ids := make([]model.FeatureID, 0, len(r.Features))
	for id := range r.Features {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

type rawEntry struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	UserFacing  *bool  `json:"userFacing"`
}

// Parse builds the registry from feature files (any *.json under FeaturesDir).
// A file may hold one object, an array of objects, or the legacy
// {"features": [...]} wrapper. A duplicated id collapses to the first file in
// path order and adds a warning naming both files.
func Parse(files Files) (Registry, error) {
	reg := Registry{Features: map[model.FeatureID]model.Feature{}}
	for _, p := range sortedPaths(files) {
		if !strings.HasSuffix(p, ".json") {
			continue
		}
		entries, err := decodeEntries(files[p])
		if err != nil {
			return reg, fmt.Errorf("%s: %w", p, err)
		}
		for _, e := range entries {
			if e.ID == "" {
				return reg, fmt.Errorf("%s: entry without an id", p)
			}
			id := model.FeatureID(e.ID)
			if prev, dup := reg.Features[id]; dup {
				if !contains(prev.Files, p) {
					reg.Warnings = append(reg.Warnings, fmt.Sprintf("duplicate id %s declared in %s and %s (collapsed to one record)", id, prev.Files[0], p))
				}
				continue
			}
			reg.Features[id] = model.Feature{ID: id, Type: e.Type, Name: e.Name, Description: e.Description, UserFacing: e.UserFacing, Files: []string{p}, InBackendDir: strings.HasPrefix(p, FeaturesDir+"/backend/")}
		}
	}
	return reg, nil
}

func decodeEntries(data []byte) ([]rawEntry, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, errors.New("empty file")
	}
	switch trimmed[0] {
	case '[':
		var arr []rawEntry
		return arr, json.Unmarshal(data, &arr)
	case '{':
		var wrapped struct {
			rawEntry
			Features []rawEntry `json:"features"`
		}
		if err := json.Unmarshal(data, &wrapped); err != nil {
			return nil, err
		}
		if wrapped.Features != nil {
			return wrapped.Features, nil
		}
		return []rawEntry{wrapped.rawEntry}, nil
	}
	return nil, errors.New("not a JSON object or array")
}

// Sidecars are the parsed sidecar files plus per-file problems.
type Sidecars struct {
	ByID     map[model.FeatureID]model.Sidecar
	Problems []string
}

// IDs returns the sidecar ids in sorted order.
func (s Sidecars) IDs() []model.FeatureID {
	ids := make([]model.FeatureID, 0, len(s.ByID))
	for id := range s.ByID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// ParseSidecars reads every *.json under SidecarDir except schema.json. A file
// whose name is not the encoding of its inner id, or a repeated id, is a problem.
func ParseSidecars(files Files) Sidecars {
	out := Sidecars{ByID: map[model.FeatureID]model.Sidecar{}}
	for _, p := range sortedPaths(files) {
		name := path.Base(p)
		if !strings.HasSuffix(name, ".json") || name == "schema.json" {
			continue
		}
		var sc model.Sidecar
		if err := json.Unmarshal(files[p], &sc); err != nil {
			out.Problems = append(out.Problems, fmt.Sprintf("%s: invalid JSON: %v", p, err))
			continue
		}
		if sc.ID == "" {
			out.Problems = append(out.Problems, fmt.Sprintf("%s: missing id", p))
			continue
		}
		if want := SidecarFilename(sc.ID); want != name {
			out.Problems = append(out.Problems, fmt.Sprintf("%s: id %q must be stored as %s", p, sc.ID, want))
		}
		if _, dup := out.ByID[sc.ID]; dup {
			out.Problems = append(out.Problems, fmt.Sprintf("%s: duplicate sidecar for id %q", p, sc.ID))
			continue
		}
		out.ByID[sc.ID] = sc
	}
	return out
}

// SidecarFilename encodes an id as <id>.json, percent-encoding only %, : and /.
func SidecarFilename(id model.FeatureID) string {
	r := strings.NewReplacer("%", "%25", ":", "%3A", "/", "%2F")
	return r.Replace(string(id)) + ".json"
}

// DecodeSidecarFilename inverts SidecarFilename.
func DecodeSidecarFilename(name string) (model.FeatureID, error) {
	stem, ok := strings.CutSuffix(name, ".json")
	if !ok {
		return "", fmt.Errorf("%q is not a .json file", name)
	}
	id, err := url.PathUnescape(stem)
	if err != nil {
		return "", fmt.Errorf("decode %q: %w", name, err)
	}
	return model.FeatureID(id), nil
}

// ReadDir reads every file under root/rel into Files keyed by repo-relative path.
// A missing directory yields no files.
func ReadDir(root, rel string) (Files, error) {
	files := Files{}
	base := filepath.Join(root, filepath.FromSlash(rel))
	err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && p == base {
				return fs.SkipAll
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relPath)] = data
		return nil
	})
	return files, err
}

// ReadBase reads every file under rel from the base tree through go-git.
func ReadBase(r *gitbase.Repo, base, rel string) (Files, error) {
	names, err := r.ListAt(base, rel)
	if err != nil {
		return nil, err
	}
	files := Files{}
	for _, n := range names {
		data, err := r.ReadBlob(base, n)
		if err != nil {
			return nil, err
		}
		files[n] = data
	}
	return files, nil
}

// WriteSidecar writes sc to its canonical filename under root; it reports
// whether the file changed (false when already identical).
func WriteSidecar(root string, sc model.Sidecar) (bool, error) {
	data, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return false, err
	}
	data = append(data, '\n')
	p := filepath.Join(root, filepath.FromSlash(SidecarDir), SidecarFilename(sc.ID))
	if old, err := os.ReadFile(p); err == nil && string(old) == string(data) {
		return false, nil
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(p, data, 0o644)
}

func sortedPaths(files Files) []string {
	ps := make([]string, 0, len(files))
	for p := range files {
		ps = append(ps, p)
	}
	sort.Strings(ps)
	return ps
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
