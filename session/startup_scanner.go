package session

import (
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection"
)

// StartupScanner scans running sessions for pre-existing approval prompts and adds
// matches to the review queue immediately, before the first regular poll cycle.
type StartupScanner struct {
	statusManager   StatusProvider
	contentProvider ContentProvider
	detector        *detection.StatusDetector
	determiner      StatusDeterminer
}

// NewStartupScanner creates a StartupScanner using the provided status and content providers.
func NewStartupScanner(statusManager StatusProvider, contentProvider ContentProvider) *StartupScanner {
	config := DefaultReviewQueuePollerConfig()
	return &StartupScanner{
		statusManager:   statusManager,
		contentProvider: contentProvider,
		detector:        detection.NewStatusDetector(),
		determiner:      NewDefaultStatusDeterminer(config),
	}
}

// Scan iterates over instances and adds any that need attention to the queue.
// Returns the number of sessions added to the queue.
func (ss *StartupScanner) Scan(instances []*Instance, queue ReviewQueueWriter) int {
	scanned, added := 0, 0
	for _, inst := range instances {
		if !inst.Started() || inst.Paused() {
			continue
		}
		scanned++

		statusInfo := ss.statusManager.GetStatus(inst)
		// nil paneActivity: startup scan has no prior #{pane_last_activity} snapshot.
		// GetContent falls back to TTL-based cache logic, which is appropriate for a
		// one-shot scan that runs before the regular poll cycle establishes a baseline.
		content := ss.contentProvider.GetContent(inst, statusInfo, nil)

		result := ss.determiner.Determine(inst, content, statusInfo, ss.detector)
		if result.Action == DetectionActionAdd {
			item := buildStartupItem(inst, result)
			queue.Add(item)
			added++
			log.InfoLog.Printf("[StartupScan] Session '%s': detected %s (status=%s)",
				inst.Title, result.Reason, result.ClaudeStatus)
		}
	}
	log.InfoLog.Printf("[StartupScan] Scanned %d sessions, added %d to review queue", scanned, added)
	return added
}

// buildStartupItem creates a ReviewItem from an instance and a DetectionResult.
func buildStartupItem(inst *Instance, result DetectionResult) *ReviewItem {
	lastActivity := inst.LastMeaningfulOutput
	if lastActivity.IsZero() {
		lastActivity = inst.CreatedAt
	}
	return &ReviewItem{
		SessionID:    inst.Title,
		SessionName:  inst.Title,
		Reason:       result.Reason,
		Priority:     result.Priority,
		DetectedAt:   time.Now(),
		Context:      result.Context,
		Program:      inst.Program,
		Branch:       inst.Branch,
		Path:         inst.Path,
		WorkingDir:   inst.WorkingDir,
		Status:       inst.Status.String(),
		Tags:         inst.Tags,
		Category:     inst.Category,
		DiffStats:    inst.GetDiffStats(),
		LastActivity: lastActivity,
		ClaudeStatus: result.ClaudeStatus,
	}
}
