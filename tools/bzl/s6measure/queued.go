package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Prober measures one terminal-stream round trip.
type Prober interface {
	Probe() (time.Duration, error)
}

var socketRE = regexp.MustCompile(`^ssq-(manual|s6)[A-Za-z0-9._-]{0,58}$`)

// ValidateIsolation refuses anything that could reach the live :8543 service or its default tmux server.
func ValidateIsolation(socket, url string) error {
	if !socketRE.MatchString(socket) {
		return fmt.Errorf("tmux socket %q must match ssq-manual* or ssq-s6* (never the live service's default server)", socket)
	}
	for _, live := range []string{":8543", ":8444"} {
		if strings.Contains(url, live) {
			return fmt.Errorf("manual instance URL %q names the live service port %s", url, live)
		}
	}
	return nil
}

// tmuxProber round-trips a unique marker through a pane on the manual instance's private tmux socket: send-keys
// into a pty running cat, then poll capture-pane until the marker is drawn. It exercises the tmux server and pty
// path a terminal stream rides on; it is not a WebSocket client (named limit).
type tmuxProber struct {
	X       Exec
	H       Host
	Sock    string
	Session string
	n       int
}

const probeSession = "s6stream"

func (p *tmuxProber) tmux(args ...string) (Out, error) {
	return p.X.Run(context.Background(), Cmd{Name: "tmux", Args: append([]string{"-L", p.Sock}, args...)})
}

func (p *tmuxProber) Setup() error {
	p.Session = probeSession
	o, err := p.tmux("new-session", "-d", "-s", p.Session, "-x", "200", "-y", "50", "cat")
	if err == nil && o.Code != 0 {
		err = fmt.Errorf("tmux new-session exit %d: %s", o.Code, strings.TrimSpace(o.Stderr))
	}
	return err
}

func (p *tmuxProber) Teardown() { _, _ = p.tmux("kill-session", "-t", p.Session) }

func (p *tmuxProber) Probe() (time.Duration, error) {
	p.n++
	marker := fmt.Sprintf("S6M%06d", p.n)
	t0 := p.H.Now()
	if o, err := p.tmux("send-keys", "-t", p.Session, marker, "Enter"); err != nil || o.Code != 0 {
		return 0, fmt.Errorf("send-keys failed")
	}
	for range 5000 {
		o, err := p.tmux("capture-pane", "-p", "-t", p.Session)
		if err == nil && strings.Contains(o.Stdout, marker) {
			return p.H.Now().Sub(t0), nil
		}
		p.H.Sleep(time.Millisecond)
	}
	return 0, fmt.Errorf("marker %s never appeared", marker)
}

// SampleN takes n probes at gap spacing and returns milliseconds.
func SampleN(p Prober, h Host, n int, gap time.Duration) ([]float64, error) {
	var ms []float64
	for range n {
		d, err := p.Probe()
		if err != nil {
			return ms, err
		}
		ms = append(ms, float64(d)/float64(time.Millisecond))
		h.Sleep(gap)
	}
	return ms, nil
}

// SampleUntil probes until done is closed.
func SampleUntil(p Prober, h Host, done <-chan struct{}, gap time.Duration) ([]float64, error) {
	var ms []float64
	for {
		select {
		case <-done:
			return ms, nil
		default:
		}
		d, err := p.Probe()
		if err != nil {
			return ms, err
		}
		ms = append(ms, float64(d)/float64(time.Millisecond))
		h.Sleep(gap)
	}
}

// QueuedOpts configures the 8-queued-invocation check.
type QueuedOpts struct {
	Target      string
	Invocations int // 8
	Worktrees   int // invocations are spread round-robin; the wrapper's slots queue them
	IdleSamples int
	Gap         time.Duration
	WatchdogGB  float64
	Sample      bool
	Prober      Prober
	Health      func() error // optional manual-instance liveness check
}

// Queued runs Story 0.10's third criterion: pre-mortem #1 live-service protection under queued load.
func Queued(h *Harness, o QueuedOpts) *Report {
	rep := NewReport()
	if o.Health != nil {
		if err := o.Health(); err != nil {
			rep.Errorf("manual instance not answering: %v", err)
			return rep
		}
	}
	idle, err := SampleN(o.Prober, h.H, o.IdleSamples, o.Gap)
	if err != nil || len(idle) == 0 {
		rep.Errorf("idle stream probe: %v", err)
		return rep
	}
	var wts []string
	defer func() {
		for _, w := range wts {
			h.DropWT(w)
		}
	}()
	for i := range o.Worktrees {
		w, err := h.AddWT(fmt.Sprintf("q%d", i), i)
		if err != nil {
			rep.Errorf("worktree: %v", err)
			return rep
		}
		wts = append(wts, w)
	}
	var sm *Sampler
	if o.Sample {
		sm = &Sampler{WatchdogKB: int64(o.WatchdogGB * 1024 * 1024)}
	}
	var loaded []float64
	var probeErr error
	failed := 0
	err = h.Gated("queued", func() error {
		done, finished := make(chan struct{}), make(chan struct{})
		go func() {
			defer close(finished)
			loaded, probeErr = SampleUntil(o.Prober, h.H, done, o.Gap)
		}()
		var wg sync.WaitGroup
		var mu sync.Mutex
		for i := range o.Invocations {
			wg.Add(1)
			go func() {
				defer wg.Done()
				w := wts[i%len(wts)]
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if sm != nil {
					if ob, e := h.OutputBase(w); e == nil {
						sm.Add(ob, cancel)
					}
				}
				out, e := h.Bazel(ctx, w, "build", h.DiskFlag(), o.Target)
				if e != nil || out.Code != 0 {
					mu.Lock()
					failed++
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		close(done)
		<-finished
		return nil
	})
	if err != nil {
		rep.Errorf("queued: %v", err)
		return rep
	}
	if probeErr != nil || len(loaded) == 0 || failed > 0 {
		rep.Errorf("queued run incomplete: %d failed invocations, %d load samples, probe error %v", failed, len(loaded), probeErr)
		return rep
	}
	idleP99, loadP99 := Percentile(idle, 99), Percentile(loaded, 99)
	rep.Keys["queued_invocations"], rep.Keys["stream_idle_p99_ms"], rep.Keys["stream_load_p99_ms"] = o.Invocations, idleP99, loadP99
	rep.Keys["stream_p99_ratio"] = loadP99 / idleP99
	rep.Check("stream_p99", idleP99 > 0 && loadP99 <= 1.2*idleP99, "stream p99 latency is at most 1.2x idle")
	if sm != nil {
		peak := float64(sm.Stop()) / 1024 / 1024
		rep.Keys["queued_peak_rss_gb"], rep.Keys["queued_watchdog_fired"] = peak, sm.Fired
		rep.Check("queued_rss", !sm.Fired && peak <= 12, "peak combined RSS is at most 12 GB")
	} else {
		rep.Errorf("queued_rss: RSS sampling off, the peak-RSS clause is not evaluated")
	}
	return rep
}
