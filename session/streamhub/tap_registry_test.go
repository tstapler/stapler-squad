package streamhub

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// fakeClock is a manually advanced clock.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// noTimer is an AfterFunc that never fires.
func noTimer(time.Duration, func()) func() bool { return nil }

// newTestRegistry returns a registry with a fake clock and no real timers.
func newTestRegistry(t *testing.T) (*TapRegistry, *fakeClock, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "tap")
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	reg := NewTapRegistry(TapRegistryOptions{
		Now:       clock.Now,
		DirFn:     func() (string, error) { return dir, nil },
		AfterFunc: noTimer,
	})
	return reg, clock, dir
}

func readTapFile(t *testing.T, path string) []TapRecord {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	recs, err := ReadTap(f)
	if err != nil {
		t.Fatal(err)
	}
	return recs
}

func TestTapRegistry_should_RecordOnLiveHandle_When_EnabledAtRuntime(t *testing.T) {
	reg, _, dir := newTestRegistry(t)
	h := reg.Handle("s")
	h.Record(TapOutput, "", []byte("before"))
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("disabled tap must not touch the disk, stat err = %v", err)
	}

	if _, err := reg.Set(true, nil, 0); err != nil {
		t.Fatal(err)
	}
	h.Record(TapOutput, "", []byte("during"))

	recs := readTapFile(t, filepath.Join(dir, "s.jsonl"))
	if len(recs) != 1 || string(recs[0].Bytes()) != "during" {
		t.Fatalf("records = %+v, want only the one written while enabled", recs)
	}
}

func TestTapRegistry_should_CloseFileAndAppendOnReenable_When_ToggledOffAndOn(t *testing.T) {
	reg, _, dir := newTestRegistry(t)
	h := reg.Handle("s")
	if _, err := reg.Set(true, []string{"s"}, 0); err != nil {
		t.Fatal(err)
	}
	h.Record(TapOutput, "", []byte("one"))
	if _, err := reg.Set(false, nil, 0); err != nil {
		t.Fatal(err)
	}
	h.s.mu.Lock()
	closed := h.s.w == nil
	h.s.mu.Unlock()
	if !closed {
		t.Fatal("disable must close the file")
	}
	h.Record(TapOutput, "", []byte("dropped"))
	if _, err := reg.Set(true, []string{"s"}, 0); err != nil {
		t.Fatal(err)
	}
	h.Record(TapOutput, "", []byte("two"))

	recs := readTapFile(t, filepath.Join(dir, "s.jsonl"))
	if len(recs) != 2 || string(recs[0].Bytes()) != "one" || string(recs[1].Bytes()) != "two" {
		t.Fatalf("records = %+v, want [one two]", recs)
	}
}

func TestTapRegistry_should_LimitRecording_When_ScopedToNamedSessions(t *testing.T) {
	reg, _, dir := newTestRegistry(t)
	a, b := reg.Handle("a"), reg.Handle("b")
	if _, err := reg.Set(true, []string{"a"}, 0); err != nil {
		t.Fatal(err)
	}
	a.Record(TapOutput, "", []byte("x"))
	b.Record(TapOutput, "", []byte("x"))
	if _, err := os.Stat(filepath.Join(dir, "b.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("session b must not be recorded, stat err = %v", err)
	}

	// A handle created after the scope was set picks up the scope too.
	late := reg.Handle("a2")
	late.Record(TapOutput, "", []byte("x"))
	if _, err := os.Stat(filepath.Join(dir, "a2.jsonl")); !os.IsNotExist(err) {
		t.Fatal("a2 was not named and must not be recorded")
	}

	if _, err := reg.Set(true, nil, 0); err != nil {
		t.Fatal(err)
	}
	b.Record(TapOutput, "", []byte("x"))
	reg.Handle("c").Record(TapOutput, "", []byte("x"))
	for _, name := range []string{"b.jsonl", "c.jsonl"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("all-sessions enable must cover %s: %v", name, err)
		}
	}
}

func TestTapRegistry_should_RejectNamedDisable_When_AllSessionsEnabled(t *testing.T) {
	reg, _, _ := newTestRegistry(t)
	if _, err := reg.Set(true, nil, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Set(false, []string{"a"}, 0); err != ErrTapAllEnabled {
		t.Fatalf("err = %v, want ErrTapAllEnabled", err)
	}
}

func TestTapRegistry_should_TurnOffAtDeadline_When_TTLExpires(t *testing.T) {
	reg, clock, dir := newTestRegistry(t)
	h := reg.Handle("s")
	st, err := reg.Set(true, nil, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if want := clock.Now().Add(time.Minute); !st.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %v, want %v", st.ExpiresAt, want)
	}
	h.Record(TapOutput, "", []byte("in"))

	clock.Advance(time.Minute)
	h.Record(TapOutput, "", []byte("out")) // the write path notices the deadline itself

	if recs := readTapFile(t, filepath.Join(dir, "s.jsonl")); len(recs) != 1 {
		t.Fatalf("records = %d, want 1: a write after the TTL must not land", len(recs))
	}
	if st := reg.Status(); st.Enabled || st.AllEnabled {
		t.Fatalf("status after expiry = %+v, want off", st)
	}
}

func TestTapRegistry_should_ExpirePerSessionScope_When_StatusIsRead(t *testing.T) {
	reg, clock, _ := newTestRegistry(t)
	if _, err := reg.Set(true, []string{"a"}, time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Set(true, []string{"b"}, time.Hour); err != nil {
		t.Fatal(err)
	}
	clock.Advance(2 * time.Minute)
	st := reg.Status()
	if len(st.SessionIDs) != 1 || st.SessionIDs[0] != "b" {
		t.Fatalf("SessionIDs = %v, want [b]", st.SessionIDs)
	}
}

func TestTapRegistry_should_ApplyDefaultAndClampTTL(t *testing.T) {
	tests := []struct {
		name string
		ttl  time.Duration
		want time.Duration
	}{
		{"zero uses default", 0, TapDefaultTTL},
		{"within range kept", time.Hour, time.Hour},
		{"over max clamped", 100 * time.Hour, TapMaxTTL},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg, clock, _ := newTestRegistry(t)
			st, err := reg.Set(true, nil, tc.ttl)
			if err != nil {
				t.Fatal(err)
			}
			if want := clock.Now().Add(tc.want); !st.ExpiresAt.Equal(want) {
				t.Fatalf("ExpiresAt = %v, want %v", st.ExpiresAt, want)
			}
		})
	}
}

// timerRecorder is an injected AfterFunc that records the single pending timer.
type timerRecorder struct {
	calls   int
	stops   int
	d       time.Duration
	f       func()
	stopped bool
}

func (tr *timerRecorder) afterFunc(d time.Duration, f func()) func() bool {
	tr.calls++
	tr.d, tr.f, tr.stopped = d, f, false
	stopped := &tr.stopped
	return func() bool { tr.stops++; *stopped = true; return true }
}

func newTimedRegistry(t *testing.T) (*TapRegistry, *fakeClock, *timerRecorder) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "tap")
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	tr := &timerRecorder{}
	reg := NewTapRegistry(TapRegistryOptions{
		Now: clock.Now, DirFn: func() (string, error) { return dir, nil }, AfterFunc: tr.afterFunc,
	})
	return reg, clock, tr
}

func TestTapRegistry_should_ArmOneTimerForEarliestDeadline_When_ScopesAreEnabled(t *testing.T) {
	reg, _, tr := newTimedRegistry(t)
	if _, err := reg.Set(true, []string{"a"}, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Set(true, []string{"b"}, 5*time.Minute); err != nil {
		t.Fatal(err)
	}
	if tr.d != 5*time.Minute {
		t.Fatalf("timer = %v, want the earliest deadline (5m)", tr.d)
	}
	if live := tr.calls - tr.stops; live != 1 {
		t.Fatalf("live timers = %d (armed %d, stopped %d), want exactly 1", live, tr.calls, tr.stops)
	}
}

func TestTapRegistry_should_TurnOffAndRearm_When_TimerCallbackFires(t *testing.T) {
	reg, clock, tr := newTimedRegistry(t)
	a, b := reg.Handle("a"), reg.Handle("b")
	if _, err := reg.Set(true, []string{"a"}, time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Set(true, []string{"b"}, time.Hour); err != nil {
		t.Fatal(err)
	}

	clock.Advance(time.Minute)
	tr.f() // the captured timer callback, as the runtime would call it

	if a.s.enabled.Load() {
		t.Fatal("a's TTL ended: the timer callback must turn it off")
	}
	if !b.s.enabled.Load() {
		t.Fatal("b's TTL has not ended and must stay on")
	}
	if tr.d != 59*time.Minute || tr.stopped {
		t.Fatalf("timer = %v (stopped %v), want re-armed for b's remaining 59m", tr.d, tr.stopped)
	}

	clock.Advance(59 * time.Minute)
	tr.f()
	if b.s.enabled.Load() || !tr.stopped && reg.stopTimer != nil {
		t.Fatal("b must be off and no timer left pending")
	}
}

func TestTapRegistry_should_ClearTimer_When_DisabledBeforeDeadline(t *testing.T) {
	reg, _, tr := newTimedRegistry(t)
	if _, err := reg.Set(true, nil, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Set(false, nil, 0); err != nil {
		t.Fatal(err)
	}
	if !tr.stopped || reg.stopTimer != nil {
		t.Fatal("disabling must stop the pending timer")
	}
}

func TestTapRegistry_should_MergeToLaterDeadline_When_NamedEnableUnderAll(t *testing.T) {
	reg, clock, _ := newTestRegistry(t)
	h := reg.Handle("s")
	if _, err := reg.Set(true, nil, time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Set(true, []string{"s"}, time.Hour); err != nil {
		t.Fatal(err)
	}
	if got, want := time.Unix(0, h.s.deadline.Load()), clock.Now().Add(time.Hour); !got.Equal(want) {
		t.Fatalf("deadline = %v, want the later one %v", got, want)
	}
	clock.Advance(2 * time.Minute)
	reg.onTimer()
	if !h.s.enabled.Load() {
		t.Fatal("the named enable outlives the all-sessions one and must keep s recording")
	}
}

func TestTapRegistry_should_DisableOnlyNamedSession_When_OthersAreEnabledByName(t *testing.T) {
	reg, _, _ := newTestRegistry(t)
	a, b := reg.Handle("a"), reg.Handle("b")
	if _, err := reg.Set(true, []string{"a", "b"}, 0); err != nil {
		t.Fatal(err)
	}
	st, err := reg.Set(false, []string{"a"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if a.s.enabled.Load() || !b.s.enabled.Load() {
		t.Fatalf("a enabled=%v b enabled=%v, want a off and b on", a.s.enabled.Load(), b.s.enabled.Load())
	}
	if len(st.SessionIDs) != 1 || st.SessionIDs[0] != "b" {
		t.Fatalf("SessionIDs = %v, want [b]", st.SessionIDs)
	}
}

func TestTapRegistry_should_BeIdempotent_When_DisablingWithNothingOn(t *testing.T) {
	reg, _, _ := newTestRegistry(t)
	for i := 0; i < 2; i++ {
		st, err := reg.Set(false, nil, 0)
		if err != nil || st.Enabled || st.AllEnabled || len(st.SessionIDs) != 0 {
			t.Fatalf("disable #%d = %+v, %v; want off and no error", i, st, err)
		}
	}
}

func TestTapRegistry_should_RotateFullFileAndClearCap_When_ReenabledAfterCap(t *testing.T) {
	reg, _, dir := newTestRegistry(t)
	h := reg.Handle("s")
	if _, err := reg.Set(true, []string{"s"}, 0); err != nil {
		t.Fatal(err)
	}
	h.s.maxBytes = 300
	for i := 0; i < 20; i++ {
		h.Record(TapOutput, "", []byte("0123456789"))
	}
	if st := reg.Status(); !st.Sessions[0].Capped {
		t.Fatalf("precondition: want capped, got %+v", st.Sessions)
	}
	path := filepath.Join(dir, "s.jsonl")
	fullSize := mustSize(t, path)

	if _, err := reg.Set(true, []string{"s"}, 0); err != nil {
		t.Fatal(err)
	}
	h.Record(TapOutput, "", []byte("fresh"))

	st := reg.Status()
	if got := st.Sessions[0]; got.Capped || !got.Enabled || !got.Rotated {
		t.Fatalf("status = %+v, want recording again with Rotated set", got)
	}
	if recs := readTapFile(t, path); len(recs) != 1 || string(recs[0].Bytes()) != "fresh" {
		t.Fatalf("new file = %+v, want just the fresh record", recs)
	}
	if got := mustSize(t, path+".old"); got != fullSize {
		t.Fatalf(".old size = %d, want the full file's %d", got, fullSize)
	}

	// A second cap and re-enable replaces .old: never more than two files.
	for i := 0; i < 20; i++ {
		h.Record(TapOutput, "", []byte("0123456789"))
	}
	if _, err := reg.Set(true, []string{"s"}, 0); err != nil {
		t.Fatal(err)
	}
	h.Record(TapOutput, "", []byte("again"))
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("files = %d (%v), want exactly 2 (.jsonl and .jsonl.old)", len(entries), err)
	}
}

func mustSize(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Size()
}

// blockingWriter blocks in Write until released, standing in for a hung disk.
type blockingWriter struct {
	entered chan struct{}
	release chan struct{}
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	close(w.entered)
	<-w.release
	return len(p), nil
}

func TestTapRegistry_should_NotBlockDisableOrStatus_When_AWriteIsStuck(t *testing.T) {
	reg, _, _ := newTestRegistry(t)
	h := reg.Handle("s")
	if _, err := reg.Set(true, []string{"s"}, 0); err != nil {
		t.Fatal(err)
	}
	h.Record(TapOutput, "", []byte("open the file"))
	bw := &blockingWriter{entered: make(chan struct{}), release: make(chan struct{})}
	closer := &closeSpy{}
	h.s.mu.Lock()
	h.s.w, h.s.closer = bw, closer
	h.s.mu.Unlock()

	writeDone := make(chan struct{})
	go func() { defer close(writeDone); h.Record(TapOutput, "", []byte("stuck")) }()
	<-bw.entered

	returned := make(chan struct{})
	go func() {
		defer close(returned)
		_ = reg.Status()
		_ = reg.Handle("other")
		if _, err := reg.Set(false, nil, 0); err != nil {
			t.Error(err)
		}
		if reg.Status().Enabled {
			t.Error("tap must report off while the write is still stuck")
		}
	}()
	select {
	case <-returned:
	case <-time.After(10 * time.Second):
		t.Fatal("Set/Status/Handle blocked behind a stuck sink write")
	}

	close(bw.release)
	<-writeDone
	if !closer.closed {
		t.Fatal("the in-flight writer must close the file once the tap is off")
	}
}

func TestTapRegistry_should_EnableAllWithoutTTL_When_EnvDirSet(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "envtap")
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	reg := NewTapRegistry(TapRegistryOptions{Now: clock.Now, EnvDir: dir, AfterFunc: noTimer})

	if st := reg.Status(); !st.AllEnabled || !st.ExpiresAt.IsZero() || st.Dir != dir {
		t.Fatalf("startup status = %+v, want all enabled, no expiry, dir %s", st, dir)
	}
	h := reg.Handle("s")
	clock.Advance(100 * time.Hour)
	h.Record(TapOutput, "", []byte("x"))
	if recs := readTapFile(t, filepath.Join(dir, "s.jsonl")); len(recs) != 1 {
		t.Fatalf("env-enabled tap must ignore the TTL, records = %d", len(recs))
	}
	if _, err := reg.Set(false, nil, 0); err != nil {
		t.Fatal(err)
	}
	if reg.Status().Enabled {
		t.Fatal("a runtime disable must turn the env-enabled tap off")
	}
}

func TestTapRegistry_should_UseStateDirTap_When_NoEnvOverride(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	reg := NewTapRegistry(TapRegistryOptions{})
	got := reg.Dir()
	if filepath.Base(got) != "tap" || filepath.Dir(got) != os.Getenv("STAPLER_SQUAD_TEST_DIR") {
		t.Fatalf("Dir = %s, want <state dir>/tap", got)
	}
}

func TestTapRegistry_should_ReportSessionStatus_When_Recording(t *testing.T) {
	reg, _, dir := newTestRegistry(t)
	h := reg.Handle("s")
	if _, err := reg.Set(true, []string{"s"}, 0); err != nil {
		t.Fatal(err)
	}
	h.Record(TapOutput, "", []byte("x"))

	st := reg.Status()
	if len(st.Sessions) != 1 {
		t.Fatalf("sessions = %+v", st.Sessions)
	}
	got := st.Sessions[0]
	if got.Name != "s" || got.Path != filepath.Join(dir, "s.jsonl") || got.BytesWritten == 0 || !got.Enabled {
		t.Fatalf("session status = %+v", got)
	}
	fi, err := os.Stat(got.Path)
	if err != nil || fi.Mode().Perm() != 0o600 || fi.Size() != got.BytesWritten {
		t.Fatalf("file: %v, err %v; want 0600 and size %d", fi, err, got.BytesWritten)
	}
	di, err := os.Stat(dir)
	if err != nil || di.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %v, err %v; want 0700", di, err)
	}
}

func TestTapRegistry_should_ReportCappedAndStopUntilReenabled_When_SizeCapReached(t *testing.T) {
	reg, _, _ := newTestRegistry(t)
	h := reg.Handle("s")
	if _, err := reg.Set(true, []string{"s"}, 0); err != nil {
		t.Fatal(err)
	}
	h.s.maxBytes = 300
	for i := 0; i < 20; i++ {
		h.Record(TapOutput, "", []byte("0123456789"))
	}
	st := reg.Status()
	if len(st.Sessions) != 1 || !st.Sessions[0].Capped || st.Sessions[0].Enabled || st.Sessions[0].BytesWritten > 300 {
		t.Fatalf("status = %+v, want capped, not enabled, <=300 bytes", st.Sessions)
	}
	// A sweep must not silently revive a capped sink.
	reg.onTimer()
	if reg.Status().Sessions[0].Enabled {
		t.Fatal("cap must hold until an explicit re-enable")
	}
}

func TestTapRegistry_should_HoldSameHandleAcrossToggles_When_StreamKeepsIt(t *testing.T) {
	reg, _, _ := newTestRegistry(t)
	h := reg.Handle("s")
	if _, err := reg.Set(true, nil, 0); err != nil {
		t.Fatal(err)
	}
	if reg.Handle("s") != h {
		t.Fatal("Handle must return the same pointer so live streams see toggles")
	}
}

func TestCaptureTap_should_RecordViaLiveHub_When_EnabledAfterHubStarted(t *testing.T) {
	reg, _, dir := newTestRegistry(t)
	hub := NewStreamHub("wired", nil, WithCaptureTap(reg.Handle("wired").As(TapSourceHub)))
	hub.resizing = true
	hub.OnRawOutput([]byte("early"))

	if _, err := reg.Set(true, []string{"wired"}, 0); err != nil {
		t.Fatal(err)
	}
	hub.OnRawOutput([]byte("late"))

	recs := readTapFile(t, filepath.Join(dir, "wired.jsonl"))
	if len(recs) != 1 || string(recs[0].Bytes()) != "late" || recs[0].Src != TapSourceHub || recs[0].Kind != TapDrop {
		t.Fatalf("records = %+v, want the single post-enable drop from the hub", recs)
	}
}
