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

// newTestRegistry returns a registry with a fake clock and no real timers.
func newTestRegistry(t *testing.T) (*TapRegistry, *fakeClock, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "tap")
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	reg := NewTapRegistry(TapRegistryOptions{
		Now:       clock.Now,
		DirFn:     func() (string, error) { return dir, nil },
		AfterFunc: func(time.Duration, func()) {},
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

func TestTapRegistry_should_ScheduleSweepAtTTL_When_Enabling(t *testing.T) {
	var got time.Duration
	reg := NewTapRegistry(TapRegistryOptions{
		DirFn:     func() (string, error) { return t.TempDir(), nil },
		AfterFunc: func(d time.Duration, _ func()) { got = d },
	})
	if _, err := reg.Set(true, nil, 5*time.Minute); err != nil {
		t.Fatal(err)
	}
	if got != 5*time.Minute {
		t.Fatalf("timer = %v, want 5m", got)
	}
}

func TestTapRegistry_should_EnableAllWithoutTTL_When_EnvDirSet(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "envtap")
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	reg := NewTapRegistry(TapRegistryOptions{Now: clock.Now, EnvDir: dir, AfterFunc: func(time.Duration, func()) {}})

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
	reg.sweep()
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
