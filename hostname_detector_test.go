package main

import (
	"context"
	"reflect"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/server"
	"go.uber.org/goleak"
)

// waitFor receives from ch, failing the test if timeout elapses first. Used
// in place of a real sleep to synchronize on a fake detectFn/resolveFn
// having run, per this repo's deterministic-fast-tests skill.
func waitFor[T any](t *testing.T, ch <-chan T, timeout time.Duration, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for %s", what)
		var zero T
		return zero
	}
}

// countingDetectFn returns a detectFn that increments calls (atomically) and
// signals cycles on every invocation, for tests that only need to know a
// cycle ran, not what it detected.
func countingDetectFn(calls *int32, cycles chan<- struct{}) func(context.Context) []string {
	return func(ctx context.Context) []string {
		atomic.AddInt32(calls, 1)
		cycles <- struct{}{}
		return nil
	}
}

func newTestDetector(detectFn func(context.Context) []string, resolveFn func(context.Context, string) []string,
	tick <-chan time.Time, events <-chan struct{}) *HostnameDetector {
	return &HostnameDetector{
		srv:          &server.Server{},
		detectFn:     detectFn,
		resolveFn:    resolveFn,
		cycleTimeout: 5 * time.Second,
		networks:     map[string][]string{},
		tick:         tick,
		events:       events,
		manual:       make(chan chan redetectCycle),
		done:         make(chan struct{}),
	}
}

func noopResolveFn(ctx context.Context, ip string) []string { return nil }

func TestHostnameDetector_Run_StartupCycleRunsImmediately(t *testing.T) {
	defer goleak.VerifyNone(t)

	cycles := make(chan struct{}, 10)
	var calls int32
	d := newTestDetector(countingDetectFn(&calls, cycles), noopResolveFn, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())

	go d.Run(ctx)
	waitFor(t, cycles, time.Second, "startup cycle")

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected 1 call after startup, got %d", got)
	}

	cancel()
	waitFor(t, (<-chan struct{})(d.done), time.Second, "Run to return")
}

func TestHostnameDetector_Run_TickTriggersAnotherCycle(t *testing.T) {
	defer goleak.VerifyNone(t)

	cycles := make(chan struct{}, 10)
	var calls int32
	tick := make(chan time.Time)
	d := newTestDetector(countingDetectFn(&calls, cycles), noopResolveFn, tick, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go d.Run(ctx)
	waitFor(t, cycles, time.Second, "startup cycle")

	tick <- time.Now()
	waitFor(t, cycles, time.Second, "tick-triggered cycle")

	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 calls (startup + tick), got %d", got)
	}

	cancel()
	waitFor(t, (<-chan struct{})(d.done), time.Second, "Run to return")
}

func TestHostnameDetector_Run_EventTriggersAnotherCycle(t *testing.T) {
	defer goleak.VerifyNone(t)

	cycles := make(chan struct{}, 10)
	var calls int32
	events := make(chan struct{})
	d := newTestDetector(countingDetectFn(&calls, cycles), noopResolveFn, nil, events)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go d.Run(ctx)
	waitFor(t, cycles, time.Second, "startup cycle")

	events <- struct{}{}
	waitFor(t, cycles, time.Second, "event-triggered cycle")

	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 calls (startup + event), got %d", got)
	}

	cancel()
	waitFor(t, (<-chan struct{})(d.done), time.Second, "Run to return")
}

func TestHostnameDetector_Run_StopsOnContextCancel(t *testing.T) {
	defer goleak.VerifyNone(t)

	d := newTestDetector(func(ctx context.Context) []string { return nil }, noopResolveFn, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())

	go d.Run(ctx)
	cancel()

	waitFor(t, (<-chan struct{})(d.done), time.Second, "Run to return after ctx cancel")
}

// TestHostnameDetector_Redetect_AddOnlyMergesNetworks covers the acceptance
// criteria in plan.md Story 2.1.1: resolveFn is re-run for every currently
// known IP on every cycle (not just newly-seen ones), and a transient
// resolution failure on first sighting never permanently excludes an IP.
func TestHostnameDetector_Redetect_AddOnlyMergesNetworks(t *testing.T) {
	srv := &server.Server{}

	var cycle int
	detectFn := func(ctx context.Context) []string {
		if cycle == 0 {
			return []string{"10.0.0.5"}
		}
		return []string{"10.0.0.5", "10.0.0.9"}
	}
	resolveFn := func(ctx context.Context, ip string) []string {
		switch {
		case ip == "10.0.0.5" && cycle == 0:
			return []string{"host.example.local"}
		case ip == "10.0.0.5" && cycle == 1:
			// Re-resolved on a later cycle: gains a second hostname.
			return []string{"host.example.local", "host2.example.local"}
		case ip == "10.0.0.9" && cycle == 0:
			// Not detected yet this cycle.
			return nil
		case ip == "10.0.0.9" && cycle == 1:
			// Newly detected this cycle; first resolution attempt fails
			// transiently (simulating a DNS hiccup right after discovery).
			return nil
		case ip == "10.0.0.9" && cycle == 2:
			// Later cycle: the earlier transient failure did not
			// permanently exclude this IP from being re-resolved.
			return []string{"other.example.local"}
		}
		return nil
	}

	d := newTestDetector(detectFn, resolveFn, nil, nil)
	d.srv = srv

	ctx := context.Background()

	d.redetect(ctx, TriggerStartup)
	if !reflect.DeepEqual(d.networks["10.0.0.5"], []string{"host.example.local"}) {
		t.Fatalf("cycle 0: networks[10.0.0.5] = %v", d.networks["10.0.0.5"])
	}
	if hn := srv.GetHostnames(); !containsString(hn, "host.example.local") {
		t.Fatalf("cycle 0: SetHostnames should include host.example.local, got %v", hn)
	}

	cycle = 1
	d.redetect(ctx, TriggerTimer)
	got := append([]string(nil), d.networks["10.0.0.5"]...)
	sort.Strings(got)
	want := []string{"host.example.local", "host2.example.local"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cycle 1: networks[10.0.0.5] = %v, want %v", got, want)
	}
	if _, ok := d.networks["10.0.0.9"]; !ok {
		t.Fatalf("cycle 1: 10.0.0.9 should be a known key even though resolution failed")
	}
	if len(d.networks["10.0.0.9"]) != 0 {
		t.Fatalf("cycle 1: 10.0.0.9 should have no hostnames yet, got %v", d.networks["10.0.0.9"])
	}

	cycle = 2
	d.redetect(ctx, TriggerTimer)
	if !reflect.DeepEqual(d.networks["10.0.0.9"], []string{"other.example.local"}) {
		t.Fatalf("cycle 2: networks[10.0.0.9] = %v, want [other.example.local] (transient failure should not permanently exclude the IP)", d.networks["10.0.0.9"])
	}
	if hn := srv.GetHostnames(); !containsString(hn, "other.example.local") {
		t.Fatalf("cycle 2: SetHostnames should include other.example.local, got %v", hn)
	}
}

// TestHostnameDetector_Redetect_TimesOutBoundedByCycleTimeout covers Task
// 2.1.1b-timeout: a resolveFn that ignores cancellation entirely would hang
// Run's whole loop forever; one that respects ctx.Done() (as
// resolveLANHostnames's threaded-through subprocess/DNS calls do) lets
// redetect return within cycleTimeout instead.
func TestHostnameDetector_Redetect_TimesOutBoundedByCycleTimeout(t *testing.T) {
	const cycleTimeout = 200 * time.Millisecond

	d := newTestDetector(
		func(ctx context.Context) []string { return []string{"10.0.0.5"} },
		func(ctx context.Context, ip string) []string {
			<-ctx.Done()
			return nil
		},
		nil, nil,
	)
	d.cycleTimeout = cycleTimeout

	start := time.Now()
	done := make(chan redetectCycle, 1)
	go func() { done <- d.redetect(context.Background(), TriggerStartup) }()

	result := waitFor(t, done, cycleTimeout+2*time.Second, "redetect to return after cycleTimeout")
	elapsed := time.Since(start)

	if elapsed > cycleTimeout+time.Second {
		t.Fatalf("redetect took %v, expected roughly cycleTimeout (%v) plus a small margin", elapsed, cycleTimeout)
	}
	if result.Trigger != TriggerStartup {
		t.Fatalf("expected Trigger=TriggerStartup even on timeout, got %v", result.Trigger)
	}
}
