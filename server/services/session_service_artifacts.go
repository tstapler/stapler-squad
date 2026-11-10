package services

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/search"
)

// ArtifactRescanner queues a transcript scan from the top for one history file.
// Implemented by *artifacts.ArtifactExtractor.
type ArtifactRescanner interface {
	RescanSession(historyFile string)
}

// ArtifactService handles the Artifacts tab's RPCs.
type ArtifactService struct {
	rescanner    ArtifactRescanner
	findInstance func(id string) *session.Instance

	// index and allInstances back the cross-session search RPCs.
	index        *search.ArtifactIndex
	allInstances func() []*session.Instance
}

// SetIndex wires the artifact index; late because the extractor that fills it is
// built after the service.
func (a *ArtifactService) SetIndex(x *search.ArtifactIndex) { a.index = x }

// SetRescanner wires the extractor; late because it is built after the service.
func (a *ArtifactService) SetRescanner(r ArtifactRescanner) { a.rescanner = r }

// RescanSessionArtifacts resets the session's scan position and queues a scan.
// A rescan is read-only, so it is allowed in any session state, archived included.
func (a *ArtifactService) RescanSessionArtifacts(
	_ context.Context,
	req *connect.Request[sessionv1.RescanSessionArtifactsRequest],
) (*connect.Response[sessionv1.RescanSessionArtifactsResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}
	if a.rescanner == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("artifact extraction is not running"))
	}
	inst := a.findInstance(req.Msg.Id)
	if inst == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}
	historyFile := inst.Snapshot().HistoryFilePath
	if historyFile == "" {
		return connect.NewResponse(&sessionv1.RescanSessionArtifactsResponse{ScanStarted: false}), nil
	}
	a.rescanner.RescanSession(historyFile)
	return connect.NewResponse(&sessionv1.RescanSessionArtifactsResponse{ScanStarted: true}), nil
}
