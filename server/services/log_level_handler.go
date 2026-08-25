package services

import (
	"encoding/json"
	"net/http"

	"github.com/tstapler/stapler-squad/log"
)

// LogLevelHandler exposes a simple REST endpoint for adjusting the server log level
// at runtime without restart. Intended for the debug menu in the web UI.
type LogLevelHandler struct{}

func NewLogLevelHandler() *LogLevelHandler { return &LogLevelHandler{} }

// RegisterRoutes wires the handler into mux.
func (h *LogLevelHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/debug/log-level", h.HandleGet)
	mux.HandleFunc("POST /api/debug/log-level", h.HandleSet)
	mux.HandleFunc("GET /api/debug/log-level/packages", h.HandleGetPackages)
	mux.HandleFunc("POST /api/debug/log-level/packages", h.HandleSetPackage)
	mux.HandleFunc("DELETE /api/debug/log-level/packages", h.HandleClearPackage)
}

type logLevelResponse struct {
	Level string `json:"level"`
}

// HandleGet returns the current global runtime log level.
func (h *LogLevelHandler) HandleGet(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(logLevelResponse{Level: log.GetRuntimeLevel().String()})
}

// HandleSet sets the global runtime log level. Body: {"level":"DEBUG"|"INFO"|"WARNING"|"ERROR"}
func (h *LogLevelHandler) HandleSet(w http.ResponseWriter, r *http.Request) {
	var req logLevelResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	level := log.ParseLogLevel(req.Level)
	log.SetRuntimeLevel(level)
	log.Info("runtime log level changed via debug API", "level", level)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(logLevelResponse{Level: level.String()})
}

type packageLevelEntry struct {
	Package string `json:"package"`
	Level   string `json:"level"`
}

// HandleGetPackages returns every configured per-package level override —
// the Java/logback-style hierarchical overrides layered on top of the
// global level (see log/package_level.go). Package paths are module-relative,
// e.g. "session/tmux".
func (h *LogLevelHandler) HandleGetPackages(w http.ResponseWriter, _ *http.Request) {
	overrides := log.GetPackageLevels()
	entries := make([]packageLevelEntry, 0, len(overrides))
	for pkg, level := range overrides {
		entries = append(entries, packageLevelEntry{Package: pkg, Level: level.String()})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(entries)
}

// HandleSetPackage sets (or replaces) one package's level override.
// Body: {"package":"session/tmux","level":"DEBUG"}
func (h *LogLevelHandler) HandleSetPackage(w http.ResponseWriter, r *http.Request) {
	var req packageLevelEntry
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Package == "" {
		http.Error(w, "invalid JSON body — want {\"package\":\"...\",\"level\":\"...\"}", http.StatusBadRequest)
		return
	}
	level := log.ParseLogLevel(req.Level)
	log.SetPackageLevel(req.Package, level)
	log.Info("per-package log level changed via debug API", "package", req.Package, "level", level)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(packageLevelEntry{Package: req.Package, Level: level.String()})
}

// HandleClearPackage removes one package's override, falling back to the
// global level (or a less-specific ancestor override). Body: {"package":"session/tmux"}
func (h *LogLevelHandler) HandleClearPackage(w http.ResponseWriter, r *http.Request) {
	var req packageLevelEntry
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Package == "" {
		http.Error(w, "invalid JSON body — want {\"package\":\"...\"}", http.StatusBadRequest)
		return
	}
	log.ClearPackageLevel(req.Package)
	log.Info("per-package log level override cleared via debug API", "package", req.Package)
	w.WriteHeader(http.StatusNoContent)
}
