package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/buildinfo"
	"github.com/tstapler/stapler-squad/pkg/changelog"
	"github.com/tstapler/stapler-squad/server/services"
)

// releasesRepo is the repo whose GitHub Releases the update check polls.
const releasesRepo = "tstapler/stapler-squad"

type changelogResponse struct {
	CurrentVersion string              `json:"current_version"`
	Branch         string              `json:"branch,omitempty"`
	Commit         string              `json:"commit,omitempty"`
	IsSourceBuild  bool                `json:"is_source_build"`
	Releases       []changelog.Release `json:"releases"`
}

// flagSnapshotTTL bounds how often config.json is re-read: the header badge
// polls /api/update-check and LoadConfig hits disk on every call.
const flagSnapshotTTL = 5 * time.Second

type changelogFlags struct{ changelog, updateCheck bool }

var changelogFlagCache struct {
	mu   sync.Mutex
	at   time.Time
	vals changelogFlags
}

func loadChangelogFlags() changelogFlags {
	c := &changelogFlagCache
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.at) < flagSnapshotTTL {
		return c.vals
	}
	cfg := config.LoadConfig()
	c.vals = changelogFlags{
		changelog:   services.ChangelogEnabled(cfg),
		updateCheck: services.UpdateCheckFlagEnabled(cfg),
	}
	c.at = time.Now()
	return c.vals
}

func changelogEnabled() bool { return loadChangelogFlags().changelog }

func updateCheckEnabled() bool {
	f := loadChangelogFlags()
	return f.changelog && f.updateCheck
}

// isSourceBuild is true for anything that is not a release binary: Makefile
// builds stamp Branch/Commit, while plain `go build` leaves Version "dev".
func isSourceBuild() bool {
	return buildinfo.Branch != "" || buildinfo.Commit != "" || !changelog.IsNumericVersion(buildinfo.Version)
}

// registerChangelogHandlers serves /api/changelog (embedded CHANGELOG.md,
// parsed once) and /api/update-check (opt-in GitHub Releases poll). Both
// return 404 while the "changelog" flag is off.
func (s *Server) registerChangelogHandlers() *services.UpdateCheckService {
	releases := changelog.Parse(changelog.Embedded())
	if releases == nil {
		releases = []changelog.Release{}
	}

	fetcher := github.NewReleaseFetcher(releasesRepo)
	updates := services.NewUpdateCheckService(services.UpdateCheckDeps{
		CurrentVersion: buildinfo.Version,
		IsSourceBuild:  isSourceBuild(),
		Enabled:        updateCheckEnabled,
		Fetch: func(ctx context.Context) (*services.ReleaseInfo, error) {
			rel, _, err := fetcher.Latest(ctx)
			if err != nil {
				return nil, err
			}
			return &services.ReleaseInfo{Version: rel.Version, URL: rel.HTMLURL}, nil
		},
	})

	s.mux.HandleFunc("GET /api/changelog", func(w http.ResponseWriter, r *http.Request) {
		if !changelogEnabled() {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, changelogResponse{
			CurrentVersion: buildinfo.Version,
			Branch:         buildinfo.Branch,
			Commit:         buildinfo.Commit,
			IsSourceBuild:  isSourceBuild(),
			Releases:       releases,
		})
	})
	s.mux.HandleFunc("GET /api/update-check", func(w http.ResponseWriter, r *http.Request) {
		if !changelogEnabled() {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, updates.Current())
	})
	return updates
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Error("changelog: encode error", "err", err)
	}
}
