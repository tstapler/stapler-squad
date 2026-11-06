package server

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	_ "github.com/tstapler/stapler-squad/gen/proto/go/session/v1" // registers the session.v1 descriptors
)

// The browser sends every server-streaming Watch* RPC through the WebSocket
// transport (web-app createSessionWatchTransport), so each must be registered
// with a StreamingWSBridge in server.go or the page logs "WebSocket connection
// ... failed" and the live view silently goes stale. WatchWorkflows and
// WatchUserPRs were missed that way.
//
// This is a source check, not a route check: it proves server.go references each
// procedure constant (so a new Watch RPC with no registration fails here), not
// that the reference reaches a mux.Handle call. It only covers session.v1 and
// server.go.
func TestEveryServerStreamingWatchRPCHasAWebSocketBridge(t *testing.T) {
	src, err := os.ReadFile("server.go")
	require.NoError(t, err)

	var watchProcedures []string
	protoregistry.GlobalFiles.RangeFilesByPackage("session.v1", func(fd protoreflect.FileDescriptor) bool {
		services := fd.Services()
		for i := 0; i < services.Len(); i++ {
			svc := services.Get(i)
			methods := svc.Methods()
			for j := 0; j < methods.Len(); j++ {
				m := methods.Get(j)
				if strings.HasPrefix(string(m.Name()), "Watch") && m.IsStreamingServer() && !m.IsStreamingClient() {
					// generated const name: <Service><Method>Procedure
					watchProcedures = append(watchProcedures, string(svc.Name())+string(m.Name())+"Procedure")
				}
			}
		}
		return true
	})
	require.NotEmpty(t, watchProcedures, "found no server-streaming Watch* RPCs; descriptor lookup is broken")

	for _, procedure := range watchProcedures {
		assert.Contains(t, string(src), "sessionv1connect."+procedure,
			"%s is a server-streaming Watch* RPC but server.go never references it; register it with a StreamingWSBridge", procedure)
	}
}
