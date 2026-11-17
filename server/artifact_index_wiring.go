package server

import (
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"

	"github.com/tstapler/stapler-squad/session/search"
)

// artifactIndexMaxWait bounds how long the initial index build waits for the
// listener; a server that never reports one (tests) still gets its index.
const artifactIndexMaxWait = 2 * time.Minute

// listenerUp is closed once the HTTP listener has bound, so the artifact index is
// built after the server can answer requests rather than before.
var listenerUp = struct {
	once sync.Once
	ch   chan struct{}
}{ch: make(chan struct{})}

func markListenerUp() { listenerUp.once.Do(func() { close(listenerUp.ch) }) }

// indexScannedArtifacts keeps the artifact index current: a finished scan replaces
// its session's documents, so a new result is searchable without a restart.
func indexScannedArtifacts(instances func() []*instance.Instance, idx *search.ArtifactIndex) func(string, *domain.SessionArtifactsBlob) {
	return func(title string, blob *domain.SessionArtifactsBlob) {
		for _, inst := range instances() {
			if snap := inst.Snapshot(); snap.Title == title && snap.UUID != "" {
				idx.Update(snap.UUID, blob)
				return
			}
		}
	}
}

// buildArtifactIndexInBackground fills idx from the stored blobs the live
// sessions carry, once ready is closed (or artifactIndexMaxWait passes).
func buildArtifactIndexInBackground(instances func() []*instance.Instance, idx *search.ArtifactIndex, ready <-chan struct{}) {
	go func() {
		select {
		case <-ready:
		case <-time.After(artifactIndexMaxWait):
		}
		for _, inst := range instances() {
			if snap := inst.Snapshot(); snap.UUID != "" && snap.Artifacts != nil {
				idx.Update(snap.UUID, snap.Artifacts)
			}
		}
	}()
}
