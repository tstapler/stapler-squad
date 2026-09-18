package streamhub

// Transport is the delivery mechanism a Subscriber uses to receive hub
// output and be torn down. Implementations decouple the hub from any
// specific wire protocol (WebSocket, ssq-mux, in-memory test double).
type Transport interface {
	// Send delivers a frame of output bytes to the subscriber. A non-nil
	// error is treated as a delivery failure and results in the subscriber
	// being evicted, the same as a slow/blocked subscriber.
	Send(data []byte) error

	// Close releases any resources held by the transport.
	Close() error
}
