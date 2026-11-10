//go:build integration

package tymux

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
	"github.com/tstapler/tymux/clients/go/gen/tymux/v1/tymuxv1connect"
	"github.com/tstapler/tymux/clients/go/udsdialer"
)

// TestTransportIsolation_ControlLatencyUnderNoisyAttach measures CapturePane latency while many
// panes stream output over Attach, for each way of wiring the Attach and control connections. It
// is a measurement, not a regression gate: opt in with TYMUXD_ISOLATION_BENCH=1 and a daemon
// binary in TYMUXD_BIN.
func TestTransportIsolation_ControlLatencyUnderNoisyAttach(t *testing.T) {
	if os.Getenv("TYMUXD_ISOLATION_BENCH") == "" {
		t.Skip("set TYMUXD_ISOLATION_BENCH=1 (and TYMUXD_BIN) to run the transport isolation measurement")
	}
	bin := findTymuxdBinary(t)
	addr := reserveLoopbackAddr(t)
	sockDir, err := os.MkdirTemp("", "tx")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	sock := filepath.Join(sockDir, "d.sock")
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "TYMUXD_ADDR="+addr, "TYMUXD_SOCKET_PATH="+sock,
		"XDG_STATE_HOME="+t.TempDir(), "RUST_LOG=warn")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	base := "http://" + addr
	require.Eventually(t, func() bool {
		_, e := os.Stat(sock)
		return e == nil
	}, 10*time.Second, 50*time.Millisecond)

	panes := 40
	if v := os.Getenv("TYMUXD_BENCH_PANES"); v != "" {
		fmt.Sscan(v, &panes)
	}
	setup := func(hc *http.Client, url string) tymuxv1connect.TymuxServiceClient {
		return tymuxv1connect.NewTymuxServiceClient(hc, url, connect.WithGRPC())
	}
	admin := setup(udsdialer.DialTCPHTTPClient(), base)
	var paneIDs []string
	for i := 0; i < panes; i++ {
		resp, e := admin.CreateSession(context.Background(), connect.NewRequest(&v1.CreateSessionRequest{
			Name: fmt.Sprintf("bench-%d", i), Command: "/bin/sh",
			Args: []string{"-c", "yes 'the quick brown fox jumps over the lazy dog 0123456789'"},
		}))
		require.NoError(t, e)
		paneIDs = append(paneIDs, resp.Msg.GetWindows()[0].GetLayout().GetPane().GetId())
	}

	type setupDef struct {
		name          string
		attach, ctrl  func() tymuxv1connect.TymuxServiceClient
		slowConsumers bool
	}
	tcp := func() tymuxv1connect.TymuxServiceClient { return setup(udsdialer.DialTCPHTTPClient(), base) }
	uds := func() tymuxv1connect.TymuxServiceClient {
		return setup(udsdialer.DialUnixHTTPClient(sock), "http://tymuxd")
	}
	shared := func(mk func() tymuxv1connect.TymuxServiceClient) (func() tymuxv1connect.TymuxServiceClient, func() tymuxv1connect.TymuxServiceClient) {
		c := mk()
		return func() tymuxv1connect.TymuxServiceClient { return c }, func() tymuxv1connect.TymuxServiceClient { return c }
	}
	var defs []setupDef
	a, c := shared(tcp)
	defs = append(defs, setupDef{name: "tcp-shared-conn", attach: a, ctrl: c})
	a, c = shared(uds)
	defs = append(defs, setupDef{name: "uds-shared-conn", attach: a, ctrl: c})
	ac := uds()
	defs = append(defs, setupDef{name: "uds-attach-conn+uds-ctrl-conn", attach: func() tymuxv1connect.TymuxServiceClient { return ac }, ctrl: uds})
	tc := tcp()
	defs = append(defs, setupDef{name: "tcp-attach-conn+uds-ctrl-conn", attach: func() tymuxv1connect.TymuxServiceClient { return tc }, ctrl: uds})
	a, c = shared(uds)
	defs = append(defs, setupDef{name: "uds-shared-conn,slow-consumers", attach: a, ctrl: c, slowConsumers: true})
	ac = uds()
	defs = append(defs, setupDef{name: "uds-split-conn,slow-consumers", attach: func() tymuxv1connect.TymuxServiceClient { return ac }, ctrl: uds, slowConsumers: true})

	for _, d := range defs {
		d := d
		t.Run(d.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			var wg sync.WaitGroup
			var bytesRead atomic.Int64
			attachClient := d.attach()
			for i, id := range paneIDs {
				stream := attachClient.Attach(ctx)
				require.NoError(t, stream.Send(&v1.AttachRequest{Payload: &v1.AttachRequest_PaneId{PaneId: id}}))
				slow := d.slowConsumers && i%2 == 0
				wg.Add(1)
				go func() {
					defer wg.Done()
					for {
						ev, e := stream.Receive()
						if e != nil {
							return
						}
						bytesRead.Add(int64(len(ev.String())))
						if slow {
							time.Sleep(200 * time.Millisecond)
						}
					}
				}()
			}
			time.Sleep(2 * time.Second)

			ctrl := d.ctrl()
			var mu sync.Mutex
			var lat []time.Duration
			var cwg sync.WaitGroup
			deadline := time.Now().Add(10 * time.Second)
			for w := 0; w < 8; w++ {
				cwg.Add(1)
				go func(w int) {
					defer cwg.Done()
					for time.Now().Before(deadline) {
						start := time.Now()
						cctx, ccancel := context.WithTimeout(ctx, 20*time.Second)
						_, e := ctrl.CapturePane(cctx, connect.NewRequest(&v1.CapturePaneRequest{PaneId: paneIDs[(w*5+len(lat))%len(paneIDs)]}))
						ccancel()
						el := time.Since(start)
						if e == nil {
							mu.Lock()
							lat = append(lat, el)
							mu.Unlock()
						}
						time.Sleep(100 * time.Millisecond)
					}
				}(w)
			}
			cwg.Wait()
			cancel()
			wg.Wait()
			sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
			pct := func(p float64) time.Duration {
				if len(lat) == 0 {
					return 0
				}
				return lat[int(float64(len(lat)-1)*p)]
			}
			t.Logf("RESULT %-34s panes=%d n=%d p50=%v p95=%v p99=%v max=%v streamed=%dMB",
				d.name, panes, len(lat), pct(.5), pct(.95), pct(.99), pct(1), bytesRead.Load()>>20)
		})
	}
}
