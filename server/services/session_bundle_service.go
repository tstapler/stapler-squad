package services

import (
	"context"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// SessionBundleService backs the SessionBundleService RPCs (RUM session
// diagnostic bundle export). ExportSessionBundle streams the ZIP to the
// browser in fixed-size BundleChunk messages; SaveSessionBundle writes it to
// a server-side directory and returns the path + SHA-256 content hash for
// backlog hand-off.
//
// Epic 1.2 (Story 1.2.1) wires the proto + registration plumbing only; the
// assembler pipeline is implemented in Epic 1.3.
type SessionBundleService struct {
	storage *session.Storage
}

// NewSessionBundleService creates a SessionBundleService.
func NewSessionBundleService(storage *session.Storage) *SessionBundleService {
	return &SessionBundleService{storage: storage}
}

// ExportSessionBundle assembles a RUMSessionBundle ZIP and streams it as
// fixed-size BundleChunk messages. Epic 1.3 implements the assembler; this
// stub returns Unimplemented so the registration and generated-code plumbing
// can be verified independently.
func (s *SessionBundleService) ExportSessionBundle(
	_ context.Context,
	_ *connect.Request[sessionv1.ExportSessionBundleRequest],
	_ *connect.ServerStream[sessionv1.BundleChunk],
) error {
	return connect.NewError(connect.CodeUnimplemented, nil)
}

// SaveSessionBundle assembles a RUMSessionBundle ZIP, writes it to
// output_dir (or the default diagnostics directory), and returns the file
// path, size, and SHA-256 content hash. Epic 1.3 implements the assembler;
// this stub returns Unimplemented.
func (s *SessionBundleService) SaveSessionBundle(
	_ context.Context,
	_ *connect.Request[sessionv1.SaveSessionBundleRequest],
) (*connect.Response[sessionv1.SaveSessionBundleResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}
