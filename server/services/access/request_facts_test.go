package access

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestRequestFactsOf_ShouldCopyTheStampedRecordAndNeverSetAReason_WhenAChainStampedOrNot(t *testing.T) {
	t.Parallel()
	for name, stamped := range map[string]bool{"stamped": true, "unstamped": false} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var got SteerRequestFacts
			path := "/t.S/M"
			var handler http.Handler = connect.NewUnaryHandler(path,
				func(ctx context.Context, req *connect.Request[emptypb.Empty]) (*connect.Response[emptypb.Empty], error) {
					got = RequestFactsOf(ctx, req)
					return connect.NewResponse(&emptypb.Empty{}), nil
				})
			if stamped {
				handler = WithRequestRecord(ListenerRemote, true)(handler)
			}
			srv := httptest.NewServer(handler)
			t.Cleanup(srv.Close)

			req := connect.NewRequest(&emptypb.Empty{})
			req.Header().Set("User-Agent", "agent")
			req.Header().Set("Origin", "http://origin.example")
			req.Header().Set("X-Forwarded-For", "192.0.2.7")
			_, err := connect.NewClient[emptypb.Empty, emptypb.Empty](srv.Client(), srv.URL+path).CallUnary(context.Background(), req)
			require.NoError(t, err)

			want := LocalWriteRefusal{Peer: got.Refusal.Peer, PeerLoopback: true}
			if stamped {
				want.Listener, want.AuthMode, want.Proxied = ListenerRemote, AuthModeRequired, true
				want.Host, want.Origin = srv.Listener.Addr().String(), "http://origin.example"
			}
			assert.Equal(t, want, got.Refusal)
			assert.Contains(t, got.Refusal.Peer, "127.0.0.1:")
			assert.Equal(t, "agent", got.UserAgent)
		})
	}
}
