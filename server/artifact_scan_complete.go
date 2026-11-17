package server

import (
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// artifactScanCompleteHandler attaches a finished artifact scan to its session and
// announces it. It never writes the session's PR: a PR URL the scanner merely found
// is not polled (ADR-004), so the PR set changes only through AttachPR callers.
func artifactScanCompleteHandler(instances func() []*instance.Instance, bus *events.EventBus) func(string, *domain.SessionArtifactsBlob) {
	return func(title string, blob *domain.SessionArtifactsBlob) {
		// Snapshot the list to avoid a data race with concurrent AddInstance calls (M-5 fix).
		for _, inst := range instances() {
			if inst.Title == title {
				inst.SetArtifacts(blob)
				bus.Publish(events.NewSessionUpdatedEvent(inst, []string{"artifacts"}))
				return
			}
		}
	}
}
