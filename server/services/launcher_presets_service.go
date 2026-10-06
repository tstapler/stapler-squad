package services

import (
	"context"
	"errors"
	"io/fs"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"

	"connectrpc.com/connect"
)

// LauncherPresetsService handles the GetLauncherPresets RPC.
type LauncherPresetsService struct{}

// NewLauncherPresetsService creates a LauncherPresetsService.
func NewLauncherPresetsService() *LauncherPresetsService {
	return &LauncherPresetsService{}
}

// +api: launcher_presets:get
//
// GetLauncherPresets reads and validates ~/.stapler-squad/launcher-presets.json fresh on
// every call (no caching) — a missing file returns an empty, error-free response, while a
// malformed file returns an empty response with load_error populated rather than a Connect
// error, so the frontend can render a specific, diagnosable message.
func (s *LauncherPresetsService) GetLauncherPresets(ctx context.Context, req *connect.Request[sessionv1.GetLauncherPresetsRequest]) (*connect.Response[sessionv1.GetLauncherPresetsResponse], error) {
	path, err := config.DefaultLauncherPresetsPath()
	if err != nil {
		log.Warn("failed to resolve launcher presets path", "err", err)
		return connect.NewResponse(&sessionv1.GetLauncherPresetsResponse{LoadError: err.Error()}), nil
	}

	file, err := config.LoadLauncherPresets(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return connect.NewResponse(&sessionv1.GetLauncherPresetsResponse{}), nil
		}
		log.Warn("failed to load launcher presets", "path", path, "err", err)
		return connect.NewResponse(&sessionv1.GetLauncherPresetsResponse{LoadError: err.Error()}), nil
	}

	presets := make([]*sessionv1.LauncherPresetProto, 0, len(file.Presets))
	for _, p := range file.Presets {
		presets = append(presets, &sessionv1.LauncherPresetProto{
			Id:          p.ID,
			Label:       p.Label,
			Argv:        p.Argv,
			Program:     p.Program,
			DefaultPath: p.DefaultPath,
		})
	}
	log.Debug("loaded launcher presets", "count", len(presets))

	return connect.NewResponse(&sessionv1.GetLauncherPresetsResponse{Presets: presets}), nil
}
