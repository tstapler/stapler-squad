package hookipc

import "context"

// Transport carries one classification request to a resident instance. *Client (Unix socket)
// is the only implementation today; the interface is the seam for others (TCP, in-process).
// An implementation must authenticate the peer itself, since the envelope carries no
// credentials: the socket's 0600 mode does that for Unix.
type Transport interface {
	Classify(ctx context.Context, envelope ClassificationEnvelope) (ClassificationReply, error)
}

// Dialer resolves the endpoint for a working directory and the Transport that reaches it.
type Dialer func(cwd string) (HookEndpoint, Transport, error)

// DialUnix is the default Dialer: it resolves the instance-scoped socket endpoint.
func DialUnix(cwd string) (HookEndpoint, Transport, error) {
	endpoint, err := ResolveEndpoint(cwd)
	if err != nil {
		return HookEndpoint{}, nil, err
	}
	client, err := NewClient(endpoint)
	if err != nil {
		return HookEndpoint{}, nil, err
	}
	return endpoint, client, nil
}

var _ Transport = (*Client)(nil)
