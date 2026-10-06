package services

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/tstapler/stapler-squad/log"
)

// piApprovalExtensionRelPath is ssq-approval.ts's location under the user's
// home directory once installed by `ssq-hooks install pi` (Story 4.1.2, a
// different in-flight epic — this handler only ever reads, never writes,
// that path). See project_plans/pi-support/implementation/plan.md, Story 2.1.2.
const piApprovalExtensionRelPath = ".pi/agent/extensions/ssq-approval.ts"

// PiExtensionStatusService reports whether the global pi approval extension
// is installed on disk. ~/.pi is on the server's filesystem, not the
// browser's, so this check must run server-side.
type PiExtensionStatusService struct{}

// NewPiExtensionStatusService creates a PiExtensionStatusService.
func NewPiExtensionStatusService() *PiExtensionStatusService {
	return &PiExtensionStatusService{}
}

type piExtensionStatusResponse struct {
	Installed bool `json:"installed"`
}

// HandlePiExtensionStatus handles GET /api/pi-extension-status, reporting
// whether ~/.pi/agent/extensions/ssq-approval.ts exists. Used by the
// pi-support flag-disable warning (Story 2.1.2): the warning modal is only
// shown when there's actually something installed to warn about.
func (s *PiExtensionStatusService) HandlePiExtensionStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	installed := false
	if home, err := os.UserHomeDir(); err == nil {
		if _, statErr := os.Stat(filepath.Join(home, piApprovalExtensionRelPath)); statErr == nil {
			installed = true
		}
	} else {
		log.Error("[PiExtensionStatusService] failed to resolve home dir", "err", err)
	}

	w.Header().Set("Content-Type", "application/json")
	if encErr := json.NewEncoder(w).Encode(piExtensionStatusResponse{Installed: installed}); encErr != nil {
		log.Error("[PiExtensionStatusService] encode error", "err", encErr)
	}
}
