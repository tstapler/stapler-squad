package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
)

type stubFilesFlags struct{ editing, rangeRead, deltaWrite, editLarge bool }

func (s *stubFilesFlags) Editing(context.Context) bool    { return s.editing }
func (s *stubFilesFlags) RangeRead(context.Context) bool  { return s.rangeRead }
func (s *stubFilesFlags) DeltaWrite(context.Context) bool { return s.deltaWrite }
func (s *stubFilesFlags) EditLarge(context.Context) bool  { return s.editLarge }

// capsHandler serves GetFileTransportCapabilities from ops behind the same
// request-record stamp the real chains apply (WithRequestRecord).
type capsHandler struct {
	sessionv1connect.UnimplementedSessionServiceHandler
	ops *FileOps
}

func (h capsHandler) GetFileTransportCapabilities(ctx context.Context, _ *connect.Request[sessionv1.GetFileTransportCapabilitiesRequest]) (*connect.Response[sessionv1.GetFileTransportCapabilitiesResponse], error) {
	return connect.NewResponse(&sessionv1.GetFileTransportCapabilitiesResponse{Capabilities: h.ops.Capabilities(ctx)}), nil
}

func capsClient(t *testing.T, ops *FileOps, listener string) sessionv1connect.SessionServiceClient {
	t.Helper()
	path, h := sessionv1connect.NewSessionServiceHandler(capsHandler{ops: ops})
	mux := http.NewServeMux()
	mux.Handle(path, h)
	srv := httptest.NewServer(WithRequestRecord(listener, true)(mux))
	t.Cleanup(srv.Close)
	return sessionv1connect.NewSessionServiceClient(srv.Client(), srv.URL)
}

func fetchCaps(t *testing.T, c sessionv1connect.SessionServiceClient) *sessionv1.FileTransportCapabilities {
	t.Helper()
	resp, err := c.GetFileTransportCapabilities(context.Background(),
		connect.NewRequest(&sessionv1.GetFileTransportCapabilitiesRequest{SessionId: "test-session"}))
	require.NoError(t, err)
	return resp.Msg.Capabilities
}

func TestCapabilities_ShouldAdvertiseOnlyWhatListenerAndFlagsServe_And_BumpCapsEpochOnFlagChange(t *testing.T) {
	for _, listener := range []string{ListenerLocal, ListenerRemote} {
		t.Run(listener, func(t *testing.T) {
			flags := &stubFilesFlags{}
			ops := NewFileOps(FilePolicy{Workspace: &fakeWorkspaceProvider{existingDir: t.TempDir()}, Flags: flags}, osRootBackend{})
			c := capsClient(t, ops, listener)

			var lastEpoch uint64
			for mask := 0; mask < 16; mask++ {
				*flags = stubFilesFlags{editing: mask&1 != 0, rangeRead: mask&2 != 0, deltaWrite: mask&4 != 0, editLarge: mask&8 != 0}
				caps := fetchCaps(t, c)

				want := []sessionv1.FileOp{sessionv1.FileOp_FILE_OP_STAT, sessionv1.FileOp_FILE_OP_READ_ALL}
				if flags.editing {
					want = append(want, sessionv1.FileOp_FILE_OP_WRITE_FULL)
				}
				assert.Equal(t, want, caps.Ops, "mask %04b", mask)
				assert.Equal(t, []sessionv1.FileWire{sessionv1.FileWire_FILE_WIRE_CONNECT_UNARY}, caps.Wires)
				assert.NotContains(t, caps.Ops, sessionv1.FileOp_FILE_OP_PATCH, "no delta write is served yet")
				assert.Equal(t, int64(1536*1024), caps.Limits.MaxWriteFullBytes)
				assert.Equal(t, int64(1<<20), caps.Limits.HashCapBytes)
				assert.Equal(t, uint32(1), caps.ProtocolVersion)

				if mask == 0 {
					lastEpoch = caps.CapsEpoch
					continue
				}
				assert.Greater(t, caps.CapsEpoch, lastEpoch, "epoch rises when the flag combination changes, mask %04b", mask)
				assert.Equal(t, caps.CapsEpoch, fetchCaps(t, c).CapsEpoch, "epoch is stable while flags are unchanged")
				lastEpoch = caps.CapsEpoch
			}
		})
	}
}

func TestCapabilities_ShouldRefuseUnadvertisedOpWithFailedPrecondition_And_ServeAdvertisedOnes(t *testing.T) {
	dir := t.TempDir()
	writeFixtureFile(t, filepath.Join(dir, "a.txt"), []byte("x\n"))
	for _, editing := range []bool{false, true} {
		flags := &stubFilesFlags{editing: editing}
		ops := NewFileOps(FilePolicy{Workspace: &fakeWorkspaceProvider{existingDir: dir}, Flags: flags}, osRootBackend{})
		advertised := map[sessionv1.FileOp]bool{}
		for _, op := range ops.Capabilities(context.Background()).Ops {
			advertised[op] = true
		}
		for op := range sessionv1.FileOp_name {
			fop := sessionv1.FileOp(op)
			if fop == sessionv1.FileOp_FILE_OP_UNSPECIFIED {
				continue
			}
			err := ops.Require(context.Background(), fop)
			if advertised[fop] {
				assert.NoError(t, err, "%s advertised (editing=%v)", fop, editing)
			} else {
				assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err), "%s unadvertised (editing=%v)", fop, editing)
			}
		}
		_, err := ops.Stat(context.Background(), "test-session", "a.txt")
		assert.NoError(t, err, "advertised STAT works")
		got, err := ops.GetContent(context.Background(), "test-session", "a.txt")
		require.NoError(t, err)
		assert.Equal(t, "x\n", got.Content, "advertised READ_ALL works")
	}
}
