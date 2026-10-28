package interceptors

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/tstapler/stapler-squad/session"
)

// sessionIDFields are the request fields that name a session.
var sessionIDFields = []protoreflect.Name{"id", "session_id"}

// NewFederatedSessionGuard rejects unary RPCs that target a session mirrored from
// a peer. Federation is read-only: peers enrol without operator approval
// (ADR-002), so forwarding a mutation would let any LAN host drive this one's
// sessions. Without the guard such an ID falls through to a misleading "not found".
func NewFederatedSessionGuard() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if msg, ok := req.Any().(proto.Message); ok {
				if hostID, found := remoteSessionHost(msg); found {
					return nil, connect.NewError(connect.CodeUnimplemented,
						fmt.Errorf("session belongs to host %s; federated sessions are read-only here — open it on its owning host", hostID))
				}
			}
			return next(ctx, req)
		}
	}
}

func remoteSessionHost(msg proto.Message) (string, bool) {
	m := msg.ProtoReflect()
	fields := m.Descriptor().Fields()
	for _, name := range sessionIDFields {
		fd := fields.ByName(name)
		if fd == nil || fd.Kind() != protoreflect.StringKind || fd.IsList() || fd.IsMap() {
			continue
		}
		if id := session.ParseFederatedSessionID(m.Get(fd).String()); !id.IsLocal() {
			return id.HostID, true
		}
	}
	return "", false
}
