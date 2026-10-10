package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
)

const (
	userThemesDirName = "themes"
	maxUserThemeBytes = 64 << 10
	maxThemeValueLen  = 200
	defaultThemeBase  = "clean"
)

var (
	themeIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	// Dotted path into the web-app's theme contract, e.g. "color.primary" or "color.statusDot.running".
	themeTokenPattern = regexp.MustCompile(`^[a-zA-Z]+(\.[a-zA-Z]+){1,2}$`)
	// Token values land in CSS custom properties; reject anything that could pull in resources or end the declaration.
	themeValueForbidden = regexp.MustCompile(`(?i)[;{}<>\\]|url\(|@import|expression\(`)
	builtinThemeBases   = map[string]bool{"matrix": true, "cyberpunk77": true, "wh40k": true, "clean": true, "light": true, "dark": true}
)

// userTheme is a runtime-loadable theme: a built-in base plus CSS-variable token overrides.
type userTheme struct {
	ID          string            `json:"id"`
	Label       string            `json:"label"`
	Description string            `json:"description,omitempty"`
	Base        string            `json:"base"`
	Tokens      map[string]string `json:"tokens"`
}

// loadUserThemes reads <dir>/*.json, skipping (and logging) files that are unreadable or invalid.
func loadUserThemes(dir string) []userTheme {
	themes := []userTheme{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return themes
	}
	for _, e := range entries {
		id := strings.TrimSuffix(e.Name(), ".json")
		if e.IsDir() || id == e.Name() || !themeIDPattern.MatchString(id) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		// Stat follows symlinks (cfgcaddy links these in) so the size cap applies to the target.
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxUserThemeBytes {
			log.Warn("user-themes: skipping file", "file", e.Name())
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			log.Warn("user-themes: read failed", "file", e.Name(), "err", err)
			continue
		}
		var t userTheme
		if err := json.Unmarshal(data, &t); err != nil {
			log.Warn("user-themes: invalid JSON", "file", e.Name(), "err", err)
			continue
		}
		themes = append(themes, sanitizeUserTheme(id, t))
	}
	sort.Slice(themes, func(i, j int) bool { return themes[i].ID < themes[j].ID })
	return themes
}

func sanitizeUserTheme(id string, t userTheme) userTheme {
	t.ID = id
	if t.Label == "" {
		t.Label = id
	}
	if !builtinThemeBases[t.Base] {
		t.Base = defaultThemeBase
	}
	clean := make(map[string]string, len(t.Tokens))
	for k, v := range t.Tokens {
		if !themeTokenPattern.MatchString(k) || len(v) > maxThemeValueLen || themeValueForbidden.MatchString(v) {
			log.Warn("user-themes: dropping token", "theme", id, "token", k)
			continue
		}
		clean[k] = v
	}
	t.Tokens = clean
	return t
}

// registerUserThemesHandler serves GET /api/themes: user themes from <configDir>/themes/*.json.
func (s *Server) registerUserThemesHandler() {
	s.mux.HandleFunc("/api/themes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		themes := []userTheme{}
		if configDir, err := config.GetConfigDir(); err == nil {
			themes = loadUserThemes(filepath.Join(configDir, userThemesDirName))
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"themes": themes}); err != nil {
			log.Error("user-themes: encode error", "err", err)
		}
	})
}
