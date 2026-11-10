// Package resolve checks that sidecar help, website and tour links point at
// real documentation, resolving against the code (DOC_FILES, the docs content
// tree, tour frontmatter) rather than against names.
package resolve

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/model"
)

// Repo-relative inputs.
const (
	DocLoaderPath = "web-app/src/lib/docs/docLoader.ts"
	HelpDir       = "web-app/public/docs"
	WebsiteDocs   = "website/src/content/docs"
	ManifestPath  = "website/scripts/content-manifest.json"
	toursPrefix   = "tours/"
)

type page struct {
	file   string // repo-relative file whose headings define the anchors
	source string // for sync-generated pages, the file a contributor edits
}

type tour struct {
	file     string
	features map[string]bool
}

// Resolver holds the doc inventory read from a checkout.
type Resolver struct {
	root     string
	docFiles map[model.DocSlug]bool
	docOK    bool
	website  map[model.WebsiteDocID]page
	tours    map[string]tour
	// Problems are integrity failures in the inputs themselves (DOC_FILES drift).
	Problems []string
	// Warnings are non-gating observations (extra help files, malformed tours).
	Warnings []string
}

// Load reads the doc inventory under root.
func Load(root string) (*Resolver, error) {
	r := &Resolver{root: root, website: map[model.WebsiteDocID]page{}, tours: map[string]tour{}}
	r.loadDocFiles()
	if err := r.loadWebsite(); err != nil {
		return nil, err
	}
	return r, nil
}

var (
	docFilesDecl  = regexp.MustCompile(`(?s)export\s+const\s+DOC_FILES\s*(?::[^=]+?)?=\s*\[(.*?)\]`)
	lineComment   = regexp.MustCompile(`//[^\n]*`)
	blockComment  = regexp.MustCompile(`(?s)/\*.*?\*/`)
	stringLiteral = regexp.MustCompile(`"([^"\n]*)"|'([^'\n]*)'`)
)

// ParseDocFiles extracts the DOC_FILES array literal. It fails when the
// declaration is missing or holds anything but string literals, so the gate
// cannot silently read a stale list.
func ParseDocFiles(src []byte) ([]model.DocSlug, error) {
	m := docFilesDecl.FindSubmatch(src)
	if m == nil {
		return nil, errors.New("DOC_FILES is no longer an `export const DOC_FILES = [...]` literal")
	}
	body := blockComment.ReplaceAllString(lineComment.ReplaceAllString(string(m[1]), ""), "")
	var slugs []model.DocSlug
	for _, lit := range stringLiteral.FindAllStringSubmatch(body, -1) {
		slugs = append(slugs, model.DocSlug(lit[1]+lit[2]))
	}
	leftover := strings.NewReplacer(",", "", " ", "", "\n", "", "\t", "", "\r", "").Replace(stringLiteral.ReplaceAllString(body, ""))
	if leftover != "" {
		return nil, fmt.Errorf("DOC_FILES is no longer an array of string literals (found %q)", leftover)
	}
	return slugs, nil
}

func (r *Resolver) loadDocFiles() {
	src, err := os.ReadFile(filepath.Join(r.root, DocLoaderPath))
	if err != nil {
		r.Problems = append(r.Problems, fmt.Sprintf("%s: %v", DocLoaderPath, err))
		return
	}
	slugs, err := ParseDocFiles(src)
	if err != nil {
		r.Problems = append(r.Problems, fmt.Sprintf("%s: %v", DocLoaderPath, err))
		return
	}
	r.docOK = true
	r.docFiles = map[model.DocSlug]bool{}
	for _, s := range slugs {
		r.docFiles[s] = true
		if _, err := os.Stat(filepath.Join(r.root, HelpDir, string(s)+".md")); err != nil {
			r.Problems = append(r.Problems, fmt.Sprintf("DOC_FILES lists %q but %s/%s.md is missing", s, HelpDir, s))
		}
	}
	entries, _ := os.ReadDir(filepath.Join(r.root, HelpDir))
	for _, e := range entries {
		if slug, ok := strings.CutSuffix(e.Name(), ".md"); ok && !r.docFiles[model.DocSlug(slug)] {
			r.Warnings = append(r.Warnings, fmt.Sprintf("%s/%s is not listed in DOC_FILES", HelpDir, e.Name()))
		}
	}
}

func (r *Resolver) loadWebsite() error {
	docsRoot := filepath.Join(r.root, filepath.FromSlash(WebsiteDocs))
	err := filepath.WalkDir(docsRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && p == docsRoot {
				return fs.SkipAll
			}
			return err
		}
		ext := filepath.Ext(p)
		if d.IsDir() || (ext != ".md" && ext != ".mdx") || strings.HasPrefix(d.Name(), "_") {
			return nil
		}
		rel, err := filepath.Rel(docsRoot, p)
		if err != nil {
			return err
		}
		id := strings.TrimSuffix(filepath.ToSlash(rel), ext)
		repoRel := WebsiteDocs + "/" + filepath.ToSlash(rel)
		r.website[model.WebsiteDocID(id)] = page{file: repoRel}
		if strings.HasPrefix(id, toursPrefix) && id != toursPrefix+"index" {
			r.loadTour(strings.TrimPrefix(id, toursPrefix), repoRel)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk %s: %w", WebsiteDocs, err)
	}
	return r.applyManifest()
}

func (r *Resolver) loadTour(id, repoRel string) {
	data, err := os.ReadFile(filepath.Join(r.root, filepath.FromSlash(repoRel)))
	if err != nil {
		r.Warnings = append(r.Warnings, fmt.Sprintf("%s: %v", repoRel, err))
		return
	}
	front, _, ok := splitFrontmatter(data)
	var fm struct {
		Tour struct {
			Features []string `yaml:"features"`
		} `yaml:"tour"`
	}
	if !ok || yaml.Unmarshal(front, &fm) != nil {
		r.Warnings = append(r.Warnings, fmt.Sprintf("%s: tour frontmatter is unreadable", repoRel))
		return
	}
	t := tour{file: repoRel, features: map[string]bool{}}
	for _, f := range fm.Tour.Features {
		t.features[f] = true
	}
	r.tours[id] = t
}

// applyManifest adds sync-generated pages (content-manifest.json) that are not
// on disk yet and records each page's source file so reports name what to edit.
func (r *Resolver) applyManifest() error {
	data, err := os.ReadFile(filepath.Join(r.root, ManifestPath))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var entries []struct {
		Source  string `json:"source"`
		Include bool   `json:"include"`
		Slug    string `json:"slug"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("%s: %w", ManifestPath, err)
	}
	for _, e := range entries {
		if !e.Include || e.Slug == "" {
			continue
		}
		id := model.WebsiteDocID(e.Slug)
		pg, onDisk := r.website[id]
		if !onDisk {
			pg = page{file: e.Source}
		}
		pg.source = e.Source
		r.website[id] = pg
	}
	return nil
}

// TourIDs returns the tour ids found on disk, sorted.
func (r *Resolver) TourIDs() []string {
	ids := make([]string, 0, len(r.tours))
	for id := range r.tours {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// TourFeatures returns the registry ids a tour lists.
func (r *Resolver) TourFeatures(tourID string) []string {
	feats := r.tours[tourID].features
	out := make([]string, 0, len(feats))
	for f := range feats {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// TourLists reports whether tour tourID exists and lists registry id.
func (r *Resolver) TourLists(tourID, id string) bool { return r.tours[tourID].features[id] }

// CheckHelp returns problems with a "slug[#anchor]" help reference.
func (r *Resolver) CheckHelp(ref string) []string {
	slug, anchor := model.SplitRef(ref)
	if !r.docOK {
		return []string{fmt.Sprintf("help %s: cannot verify, DOC_FILES could not be read", ref)}
	}
	if !r.docFiles[model.DocSlug(slug)] {
		return []string{fmt.Sprintf("help %s: %s: not in DOC_FILES (add it to %s and create %s/%s.md)", ref, slug, DocLoaderPath, HelpDir, slug)}
	}
	return r.checkAnchor("help", ref, anchor, HelpDir+"/"+slug+".md", "")
}

// CheckWebsite returns problems with a "docs-id[#anchor]" website reference.
func (r *Resolver) CheckWebsite(ref string) []string {
	id, anchor := model.SplitRef(ref)
	pg, ok := r.website[model.WebsiteDocID(id)]
	if !ok {
		return []string{fmt.Sprintf("website %s: no docs page with id %q under %s", ref, id, WebsiteDocs)}
	}
	return r.checkAnchor("website", ref, anchor, pg.file, pg.source)
}

// CheckTour returns problems with a tour link from feature id: the tour must
// exist and list the id in tour.features.
func (r *Resolver) CheckTour(tourID string, id model.FeatureID) []string {
	t, ok := r.tours[tourID]
	if !ok {
		return []string{fmt.Sprintf("tour %s: no %s%s.md(x) under %s", tourID, toursPrefix, tourID, WebsiteDocs)}
	}
	if !t.features[string(id)] {
		return []string{fmt.Sprintf("tour %s: tour.features in %s does not list %s", tourID, t.file, id)}
	}
	return nil
}

func (r *Resolver) checkAnchor(kind, ref, anchor, file, source string) []string {
	if anchor == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(r.root, filepath.FromSlash(file)))
	if err != nil && source != "" {
		file = source
		data, err = os.ReadFile(filepath.Join(r.root, filepath.FromSlash(source)))
	}
	if err != nil {
		return []string{fmt.Sprintf("%s %s: cannot read %s to check anchor %s", kind, ref, file, anchor)}
	}
	if Anchors(Headings(data))[anchor] {
		return nil
	}
	hint := file
	if source != "" {
		hint = source + " (generated page; edit the source, then run node website/scripts/sync-content.mjs)"
	}
	return []string{fmt.Sprintf("%s %s: anchor %s not found in %s", kind, ref, anchor, hint)}
}
