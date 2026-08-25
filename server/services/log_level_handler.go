package services

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/tstapler/stapler-squad/log"
)

// maxPackageLevelBodyBytes bounds the request body for the per-package
// log-level endpoints — the payload is a single {"package":..,"level":..}
// object, so 4KB is generous.
const maxPackageLevelBodyBytes = 4 << 10

// auditSensitivePackages lists packages whose Info/Warn output forms a
// security-relevant audit trail (login, passkey registration, session
// invalidation) that must not be silenced via the per-package debug API —
// only via the pre-existing, less-surgical global level toggle.
var auditSensitivePackages = []string{"server/auth"} //nolint:gochecknoglobals

func isAuditSensitive(pkg string) bool {
	for _, p := range auditSensitivePackages {
		if pkg == p || strings.HasPrefix(pkg, p+"/") {
			return true
		}
	}
	return false
}

// validLogLevelNames are the strings log.ParseLogLevel accepts. Listed here
// so HandleSetPackage can reject an unrecognized level with 400 instead of
// silently coercing it to INFO (log.ParseLogLevel's default).
var validLogLevelNames = map[string]bool{ //nolint:gochecknoglobals
	"DEBUG": true, "INFO": true, "WARNING": true, "WARN": true, "ERROR": true, "FATAL": true,
}

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
	sort.Slice(entries, func(i, j int) bool { return entries[i].Package < entries[j].Package })
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(entries)
}

// HandleSetPackage sets (or replaces) one package's level override.
// Body: {"package":"session/tmux","level":"DEBUG"}
func (h *LogLevelHandler) HandleSetPackage(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPackageLevelBodyBytes)
	var req packageLevelEntry
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Package == "" {
		http.Error(w, "invalid JSON body — want {\"package\":\"...\",\"level\":\"...\"}", http.StatusBadRequest)
		return
	}
	if !validLogLevelNames[strings.ToUpper(req.Level)] {
		http.Error(w, "invalid level — want one of DEBUG, INFO, WARNING, ERROR, FATAL", http.StatusBadRequest)
		return
	}
	level := log.ParseLogLevel(req.Level)
	if isAuditSensitive(req.Package) && level > log.WARNING {
		http.Error(w, "cannot raise "+req.Package+" above WARNING via the per-package API — it carries a security-relevant audit trail; use the global log level instead", http.StatusForbidden)
		return
	}
	log.SetPackageLevel(req.Package, level)
	log.Info("per-package log level changed via debug API", "package", req.Package, "level", level, "remote_addr", r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(packageLevelEntry{Package: req.Package, Level: level.String()})
}

// HandleClearPackage removes one package's override, falling back to the
// global level (or a less-specific ancestor override). Body: {"package":"session/tmux"}
func (h *LogLevelHandler) HandleClearPackage(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPackageLevelBodyBytes)
	var req packageLevelEntry
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Package == "" {
		http.Error(w, "invalid JSON body — want {\"package\":\"...\"}", http.StatusBadRequest)
		return
	}
	log.ClearPackageLevel(req.Package)
	log.Info("per-package log level override cleared via debug API", "package", req.Package, "remote_addr", r.RemoteAddr)
	w.WriteHeader(http.StatusNoContent)
}
