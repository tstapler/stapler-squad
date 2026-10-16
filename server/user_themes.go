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
	maxUserThemes     = 50
	maxThemeTokens    = 300
	defaultThemeBase  = "clean"
)

var (
	themeIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	// Dotted path into the web-app's theme contract, e.g. "color.primary" or "color.statusDot.running".
	themeTokenPattern = regexp.MustCompile(`^[a-zA-Z]+(\.[a-zA-Z]+){1,2}$`)
	// Token values land in CSS custom properties. Allowlist the characters, and below the CSS functions,
	// so nothing can end the declaration or fetch a resource (url(), image-set(), src(), @import...).
	themeValueChars   = regexp.MustCompile(`^[a-zA-Z0-9#%.,()\s/'"_+*-]+$`)
	themeFuncCall     = regexp.MustCompile(`([a-zA-Z0-9_-]+)\(`)
	themeAllowedFuncs = map[string]bool{
		"rgb": true, "rgba": true, "hsl": true, "hsla": true, "hwb": true, "lab": true, "lch": true,
		"oklab": true, "oklch": true, "color-mix": true, "var": true, "calc": true, "min": true, "max": true, "clamp": true,
	}
	builtinThemeBases = map[string]bool{"matrix": true, "cyberpunk77": true, "wh40k": true, "clean": true, "light": true, "dark": true}
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
		if len(themes) >= maxUserThemes {
			log.Warn("user-themes: too many theme files, ignoring the rest", "max", maxUserThemes)
			break
		}
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

func safeThemeValue(v string) bool {
	if !themeValueChars.MatchString(v) {
		return false
	}
	for _, m := range themeFuncCall.FindAllStringSubmatch(v, -1) {
		if !themeAllowedFuncs[strings.ToLower(m[1])] {
			return false
		}
	}
	return true
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
		if len(clean) >= maxThemeTokens {
			log.Warn("user-themes: too many tokens, ignoring the rest", "theme", id, "max", maxThemeTokens)
			break
		}
		if !themeTokenPattern.MatchString(k) || len(v) > maxThemeValueLen || !safeThemeValue(v) {
			log.Warn("user-themes: dropping token", "theme", id, "token", k)
			continue
		}
		clean[k] = v
	}
	t.Tokens = clean
	return t
}

// mergeUserThemes loads each dir in order; a theme id in a later dir replaces the same id from an earlier one.
func mergeUserThemes(dirs ...string) []userTheme {
	byID := map[string]userTheme{}
	seen := map[string]bool{}
	for _, dir := range dirs {
		if clean := filepath.Clean(dir); seen[clean] {
			continue
		} else {
			seen[clean] = true
		}
		for _, t := range loadUserThemes(dir) {
			byID[t.ID] = t
		}
	}
	merged := make([]userTheme, 0, len(byID))
	for _, t := range byID {
		merged = append(merged, t)
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].ID < merged[j].ID })
	if len(merged) > maxUserThemes {
		log.Warn("user-themes: too many themes, keeping the first by id", "max", maxUserThemes, "found", len(merged))
		merged = merged[:maxUserThemes]
	}
	return merged
}

// globalThemesDir is ~/.stapler-squad/themes, shared by every workspace. It is skipped for isolated
// instances (test mode, STAPLER_SQUAD_INSTANCE, STAPLER_SQUAD_TEST_DIR) so they never read the real home.
func globalThemesDir() string {
	return globalThemesDirFor(config.IsIsolatedInstance())
}

func globalThemesDirFor(isolated bool) string {
	if isolated {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".stapler-squad", userThemesDirName)
}

// registerUserThemesHandler serves GET /api/themes: the global themes dir merged with <configDir>/themes
// (the workspace/instance dir wins on a matching id).
func (s *Server) registerUserThemesHandler() {
	s.mux.HandleFunc("/api/themes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var dirs []string
		if global := globalThemesDir(); global != "" {
			dirs = append(dirs, global)
		}
		if configDir, err := config.GetConfigDir(); err != nil {
			log.Warn("user-themes: config dir unavailable", "err", err)
		} else {
			dirs = append(dirs, filepath.Join(configDir, userThemesDirName))
		}
		themes := mergeUserThemes(dirs...)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"themes": themes}); err != nil {
			log.Error("user-themes: encode error", "err", err)
		}
	})
}
