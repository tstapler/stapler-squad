package server

import (
	"fmt"

	"github.com/tstapler/stapler-squad/server/prwatch"
	"github.com/tstapler/stapler-squad/session/domain"
)

// prwatchSnapshots implements prwatch.PRSnapshotSource over live session
// snapshots. It makes no network call.
type prwatchSnapshots struct {
	snapshots func() []*domain.InstanceSnapshot
}

var _ prwatch.PRSnapshotSource = (*prwatchSnapshots)(nil)

func newPRWatchSnapshots(snapshots func() []*domain.InstanceSnapshot) *prwatchSnapshots {
	return &prwatchSnapshots{snapshots: snapshots}
}

func (s *prwatchSnapshots) Snapshot(sessionUUID string) (prwatch.PRSnapshot, error) {
	for _, sn := range s.snapshots() {
		if sn == nil || sn.UUID != sessionUUID {
			continue
		}
		gh := sn.GitHub
		return prwatch.PRSnapshot{
			CheckConclusion:  sn.ActiveCheckConclusion(),
			ChangesRequested: gh.GitHubChangesReqCount,
			Terminal:         gh.GitHubPRStatusTerminal,
			CheckedAt:        gh.LastPRStatusCheck,
		}, nil
	}
	return prwatch.PRSnapshot{}, fmt.Errorf("prwatch: unknown session %q", sessionUUID)
}
