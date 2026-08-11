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
}

type logLevelResponse struct {
	Level string `json:"level"`
}

// HandleGet returns the current runtime log level.
func (h *LogLevelHandler) HandleGet(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(logLevelResponse{Level: log.GetRuntimeLevel().String()})
}

// HandleSet sets the runtime log level. Body: {"level":"DEBUG"|"INFO"|"WARNING"|"ERROR"}
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
