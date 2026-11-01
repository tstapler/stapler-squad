package services

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

// Terminal mux instruments (ADR-033). Labels are bounded: direction (2), frame type (8),
// close reason (a fixed set).
var (
	muxSocketsOpen   = upDownNamed("terminal_mux_sockets_open", "Terminal mux sockets currently open")
	muxStreamsOpen   = upDownNamed("terminal_mux_streams_open", "Terminal streams currently carried by mux sockets")
	muxFramesTotal   = counterNamed("terminal_mux_frames_total", "Terminal mux frames by direction and type")
	muxSocketsClosed = counterNamed("terminal_mux_socket_closed_total", "Terminal mux sockets ended, by reason")
	muxCreditStalls  = counterNamed("terminal_mux_credit_stalls_total", "Times a stream with queued output had to wait for client credit")
	muxWriteErrors   = counterNamed("terminal_mux_write_errors_total", "Mux socket write failures (each ends its socket)")
	muxHolWait       = histNamed("terminal_mux_hol_wait", "ms", "Time from a message being queued to its first frame being written")
	muxQueueBytes    = sync.OnceValue(func() metric.Float64Histogram {
		h, err := telemetry.GetMeter().Float64Histogram("terminal_mux_queue_bytes",
			metric.WithDescription("Per-stream unsent output bytes observed at each enqueue"),
			metric.WithExplicitBucketBoundaries(1024, 4096, 16384, 65536, 262144, 1048576))
		if err != nil {
			log.Error("services: failed to register terminal_mux_queue_bytes", "error", err)
			return nil
		}
		return h
	})
)

var muxFrameNames = map[muxFrameType]string{
	muxFrameHello: "hello", muxFrameOpen: "open", muxFrameData: "data", muxFrameCredit: "credit",
	muxFrameClose: "close", muxFramePing: "ping", muxFramePong: "pong", muxFrameConfig: "config",
}

type muxMetricsRecorder struct{}

// muxMetrics is the process-wide recorder; every method tolerates unregistered instruments.
var muxMetrics muxMetricsRecorder

func (muxMetricsRecorder) socketOpened() { addUpDown(muxSocketsOpen(), 1) }
func (muxMetricsRecorder) streamOpened() { addUpDown(muxStreamsOpen(), 1) }
func (muxMetricsRecorder) streamClosed() { addUpDown(muxStreamsOpen(), -1) }

func (muxMetricsRecorder) socketClosed(reason string) {
	addUpDown(muxSocketsOpen(), -1)
	if c := muxSocketsClosed(); c != nil {
		c.Add(context.Background(), 1, metric.WithAttributes(attribute.String("reason", reason)))
	}
}

func (muxMetricsRecorder) frame(dir string, t muxFrameType) {
	name, ok := muxFrameNames[t]
	if !ok {
		name = "other"
	}
	if c := muxFramesTotal(); c != nil {
		c.Add(context.Background(), 1, metric.WithAttributes(attribute.String("dir", dir), attribute.String("type", name)))
	}
}

func (muxMetricsRecorder) creditStall() {
	if c := muxCreditStalls(); c != nil {
		c.Add(context.Background(), 1)
	}
}

func (muxMetricsRecorder) writeError() {
	if c := muxWriteErrors(); c != nil {
		c.Add(context.Background(), 1)
	}
}

func (muxMetricsRecorder) holWait(d time.Duration) {
	if h := muxHolWait(); h != nil {
		h.Record(context.Background(), float64(d)/float64(time.Millisecond))
	}
}

func (muxMetricsRecorder) queueDepth(bytes int) {
	if h := muxQueueBytes(); h != nil {
		h.Record(context.Background(), float64(bytes))
	}
}

func addUpDown(g metric.Int64UpDownCounter, delta int64) {
	if g != nil {
		g.Add(context.Background(), delta)
	}
}
