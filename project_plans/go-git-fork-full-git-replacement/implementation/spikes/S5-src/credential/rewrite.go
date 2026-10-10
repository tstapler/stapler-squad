package credential

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/config"
	format "github.com/go-git/go-git/v5/plumbing/format/config"
)

// Rewriter applies url.<base>.insteadOf / pushInsteadOf across system, global
// and repo-local config (go-git only applies the repo's own storer config and
// keeps one insteadOf value per base).
type Rewriter struct {
	instead []rule
	push    []rule
}

type rule struct{ base, prefix string }

// LoadRewriter reads files in git precedence order. Include paths are
// followed (depth 10); includeIf is NOT supported and is reported.
func LoadRewriter(files ...string) (r *Rewriter, unsupported []string) {
	r = &Rewriter{}
	for _, f := range files {
		r.load(f, 0, &unsupported)
	}
	return r, unsupported
}

func (r *Rewriter) load(path string, depth int, unsupported *[]string) {
	if depth > 10 {
		return
	}
	data, err := os.Open(path)
	if err != nil {
		return
	}
	defer data.Close()
	cfg, err := config.ReadConfig(data)
	if err != nil {
		return
	}
	raw := cfg.Raw
	for _, ss := range raw.Section("url").Subsections {
		for _, v := range ss.Options.GetAll("insteadOf") {
			r.instead = append(r.instead, rule{ss.Name, v})
		}
		for _, v := range ss.Options.GetAll("pushInsteadOf") {
			r.push = append(r.push, rule{ss.Name, v})
		}
	}
	for _, ss := range raw.Section("includeIf").Subsections {
		*unsupported = append(*unsupported, "includeIf."+ss.Name)
	}
	for _, inc := range raw.Section("include").Options.GetAll("path") {
		if strings.HasPrefix(inc, "~/") {
			h, _ := os.UserHomeDir()
			inc = filepath.Join(h, inc[2:])
		} else if !filepath.IsAbs(inc) {
			inc = filepath.Join(filepath.Dir(path), inc)
		}
		r.load(inc, depth+1, unsupported)
	}
}

var _ = format.Config{}

func longest(rules []rule, url string) (rule, bool) {
	var best rule
	found := false
	for _, ru := range rules {
		if strings.HasPrefix(url, ru.prefix) && (!found || len(ru.prefix) > len(best.prefix)) {
			best, found = ru, true
		}
	}
	return best, found
}

// Fetch returns the URL used for fetch (insteadOf only).
func (r *Rewriter) Fetch(url string) string {
	if ru, ok := longest(r.instead, url); ok {
		return ru.base + strings.TrimPrefix(url, ru.prefix)
	}
	return url
}

// Push returns the URL used for push: pushInsteadOf if any matches, else insteadOf.
func (r *Rewriter) Push(url string) string {
	if ru, ok := longest(r.push, url); ok {
		return ru.base + strings.TrimPrefix(url, ru.prefix)
	}
	return r.Fetch(url)
}
