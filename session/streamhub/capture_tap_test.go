package streamhub

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// steppedTap returns a tap writing to buf whose clock advances 10ms per record.
func steppedTap(buf *bytes.Buffer) *CaptureTap {
	tap := newCaptureTapWriter(buf)
	tick := time.Unix(1000, 0)
	tap.s.now = func() time.Time { tick = tick.Add(10 * time.Millisecond); return tick }
	return tap
}

func TestCaptureTap_should_WriteNothingAndAllocateNothing_When_TapDisabled(t *testing.T) {
	reg := NewTapRegistry(TapRegistryOptions{DirFn: func() (string, error) { return t.TempDir(), nil }})
	handle := reg.Handle("corp-compute-nop-pr-534")
	if handle == nil {
		t.Fatal("a handle must be non-nil even while the tap is off")
	}

	hub := NewStreamHub("corp-compute-nop-pr-534", nil, WithCaptureTap(handle.As(TapSourceHub)))
	hub.resizing = true // drop path never touches the BatchWindow, so only tap cost is measured
	data := []byte("\x1b[2Kfoo")
	if allocs := testing.AllocsPerRun(100, func() { hub.OnRawOutput(data) }); allocs != 0 {
		t.Fatalf("OnRawOutput allocs with tap disabled = %v, want 0", allocs)
	}
}

func TestCaptureTap_should_RecordDropWithCause_When_ResizingDropsFrame(t *testing.T) {
	var buf bytes.Buffer
	hub := NewStreamHub("s", nil, WithCaptureTap(steppedTap(&buf)))
	hub.resizing = true

	hub.OnRawOutput([]byte("\x1b[2Kfoo"))

	got := buf.String()
	for _, want := range []string{`"t_ns":`, `"kind":"drop"`, `"cause":"resize_settling"`, `"b64":"G1syS2Zvbw=="`} {
		if !strings.Contains(got, want) {
			t.Fatalf("record %q missing %s", got, want)
		}
	}
	recs, err := ReadTap(strings.NewReader(got))
	if err != nil || len(recs) != 1 {
		t.Fatalf("ReadTap = %v, %v; want 1 record", recs, err)
	}
	if recs[0].Kind != TapDrop || recs[0].Cause != DropCauseResizeSettling || string(recs[0].Bytes()) != "\x1b[2Kfoo" {
		t.Fatalf("round trip mismatch: %+v", recs[0])
	}
}

func TestCaptureTap_should_RecordUndeliveredDrop_When_SubscriberQueueIsFull(t *testing.T) {
	var buf bytes.Buffer
	hub := NewStreamHub("s", nil, WithCaptureTap(steppedTap(&buf)), WithSlowSubscriberGrace(time.Hour))
	full := newSubscriber(SubscriberID("full"), noopTransport{}, SubscriberCapability{}, 1) // no writer started, so the queue never drains
	if !full.trySend([]byte("fills the queue")) {
		t.Fatal("first frame should fit the queue")
	}

	hub.deliver(full, []byte("lost"), time.Hour)

	recs, err := ReadTap(&buf)
	if err != nil || len(recs) != 1 {
		t.Fatalf("ReadTap = %v, %v; want 1 record", recs, err)
	}
	if recs[0].Kind != TapDrop || recs[0].Cause != DropCauseSubscriberUndelivered || string(recs[0].Bytes()) != "lost" {
		t.Fatalf("undelivered record: %+v", recs[0])
	}
}

func TestCaptureTap_should_TagRecordsWithSource_When_ViewedAsHubAndLegacy(t *testing.T) {
	var buf bytes.Buffer
	base := steppedTap(&buf)
	base.As(TapSourceHub).Record(TapOutput, "", []byte("h"))
	base.As(TapSourceLegacy).Record(TapOutput, "", []byte("l"))
	base.Record(TapOutput, "", []byte("n"))

	recs, err := ReadTap(&buf)
	if err != nil || len(recs) != 3 {
		t.Fatalf("ReadTap = %v, %v; want 3 records", recs, err)
	}
	if recs[0].Src != TapSourceHub || recs[1].Src != TapSourceLegacy || recs[2].Src != "" {
		t.Fatalf("sources = %q %q %q", recs[0].Src, recs[1].Src, recs[2].Src)
	}
	if (*CaptureTap)(nil).As(TapSourceHub) != nil {
		t.Fatal("As on a nil tap must stay nil")
	}
}

func TestCaptureTap_should_RecordOutputSnapshotAndResize_When_Delivered(t *testing.T) {
	var buf bytes.Buffer
	tap := steppedTap(&buf)
	tap.Record(TapOutput, "", []byte("abc"))
	tap.RecordResize(TerminalSize{cols: 202, rows: 47})
	tap.Record(TapSnapshot, "", []byte("snap"))

	recs, err := ReadTap(&buf)
	if err != nil || len(recs) != 3 {
		t.Fatalf("ReadTap = %v, %v; want 3 records", recs, err)
	}
	if recs[0].Kind != TapOutput || string(recs[0].Bytes()) != "abc" || recs[0].Cause != "" {
		t.Fatalf("output record: %+v", recs[0])
	}
	if recs[1].Kind != TapResize || recs[1].Cols != 202 || recs[1].Rows != 47 {
		t.Fatalf("resize record: %+v", recs[1])
	}
	if recs[2].Kind != TapSnapshot || recs[2].TNs <= recs[0].TNs {
		t.Fatalf("snapshot record: %+v", recs[2])
	}
}

// envRegistry is a registry started the way STAPLER_SQUAD_CAPTURE_TAP_DIR=dir starts it.
func envRegistry(dir string) *TapRegistry {
	return NewTapRegistry(TapRegistryOptions{EnvDir: dir})
}

func TestCaptureTap_should_CreateOwnerOnlyDirAndFile_When_EnvVarSet(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tap")

	tap := envRegistry(dir).Handle("sess")
	tap.Record(TapOutput, "", []byte("x"))

	di, err := os.Stat(dir)
	if err != nil || di.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %v, err = %v; want 0700", di.Mode().Perm(), err)
	}
	fi, err := os.Stat(filepath.Join(dir, "sess.jsonl"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %v, err = %v; want 0600", fi.Mode().Perm(), err)
	}
}

func TestCaptureTap_should_TightenExistingDirAndFileModes_When_TheyAreWorldReadable(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "sess.jsonl")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	envRegistry(dir).Handle("sess").Record(TapOutput, "", []byte("x"))
	di, derr := os.Stat(dir)
	fi, ferr := os.Stat(path)
	if derr != nil || ferr != nil {
		t.Fatalf("stat errors: dir %v, file %v", derr, ferr)
	}
	if di.Mode().Perm() != 0o700 || fi.Mode().Perm() != 0o600 {
		t.Fatalf("dir = %v, file = %v; want 0700 and 0600", di.Mode().Perm(), fi.Mode().Perm())
	}
}

func TestCaptureTap_should_RefuseSymlink_When_TapFilePathIsASymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "sess.jsonl")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	reg := envRegistry(dir)
	tap := reg.Handle("sess")
	tap.Record(TapOutput, "", []byte("x"))
	if st := reg.Status(); len(st.Sessions) != 1 || !st.Sessions[0].Failed {
		t.Fatalf("tap must refuse to follow a symlink and report failure, status = %+v", st)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "keep" {
		t.Fatalf("symlink target = %q, err = %v; want untouched", got, err)
	}
}

func TestCaptureTap_should_ReportFailureAndRetryOnReenable_When_DirIsUnwritable(t *testing.T) {
	parent := t.TempDir()
	blocker := filepath.Join(parent, "notadir")
	if err := os.WriteFile(blocker, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	reg := envRegistry(filepath.Join(blocker, "tap")) // MkdirAll fails: parent is a file
	tap := reg.Handle("sess")
	tap.Record(TapOutput, "", []byte("x"))
	if st := reg.Status(); len(st.Sessions) != 1 || !st.Sessions[0].Failed {
		t.Fatalf("status = %+v, want one failed session", st)
	}

	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Set(true, []string{"sess"}, 0); err != nil {
		t.Fatal(err)
	}
	tap.Record(TapOutput, "", []byte("y"))
	if st := reg.Status(); len(st.Sessions) != 1 || st.Sessions[0].Failed || st.Sessions[0].BytesWritten == 0 {
		t.Fatalf("re-enable after fixing the path must record: %+v", st)
	}
}

func TestCaptureTap_should_StopAndCloseFile_When_SizeCapIsReached(t *testing.T) {
	var buf bytes.Buffer
	tap := steppedTap(&buf)
	tap.s.maxBytes = 200
	closer := &closeSpy{}
	tap.s.closer = closer

	for i := 0; i < 20; i++ {
		tap.Record(TapOutput, "", []byte("0123456789"))
	}

	if int64(buf.Len()) > 200 {
		t.Fatalf("wrote %d bytes, cap is 200", buf.Len())
	}
	if !closer.closed {
		t.Fatal("file must be closed when the cap is reached")
	}
	recs, err := ReadTap(&buf)
	if err != nil {
		t.Fatalf("capped file must end on a whole line: %v", err)
	}
	if len(recs) == 0 || len(recs) >= 20 {
		t.Fatalf("records = %d, want some but fewer than 20", len(recs))
	}
}

type closeSpy struct{ closed bool }

func (c *closeSpy) Close() error { c.closed = true; return nil }

func TestCaptureTap_should_NotInterleaveLines_When_WrittenConcurrently(t *testing.T) {
	var buf bytes.Buffer
	tap := steppedTap(&buf)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				tap.Record(TapOutput, "", []byte("payload"))
			}
		}()
	}
	wg.Wait()

	recs, err := ReadTap(&buf)
	if err != nil || len(recs) != 400 {
		t.Fatalf("ReadTap = %d records, err = %v; want 400 whole records", len(recs), err)
	}
}

func TestCaptureTap_should_UseDistinctFiles_When_SanitizedNamesCollide(t *testing.T) {
	dir := t.TempDir()
	reg := envRegistry(dir)

	a, b := reg.Handle("a/b"), reg.Handle("a b")
	if a.s == b.s {
		t.Fatal("names that sanitize alike must not share a tap")
	}
	if got := reg.Handle("a/b"); got != a {
		t.Fatal("the same session name must reuse its tap")
	}
	a.Record(TapOutput, "", []byte("1"))
	b.Record(TapOutput, "", []byte("2"))
	if sanitizeTapName("plain-name_1.x") != "plain-name_1.x" {
		t.Fatal("an already-safe name must be left unchanged")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("files = %d, want 2", len(entries))
	}
}

func TestPairSettleLatency_should_PairResizeWithNextSnapshot_When_SyntheticTapRead(t *testing.T) {
	const tap = `{"t_ns":1000000000,"kind":"output","b64":"YQ=="}
{"t_ns":2000000000,"kind":"resize","cols":202,"rows":47}
{"t_ns":2100000000,"kind":"drop","cause":"resize_settling","b64":"YQ=="}
{"t_ns":2500000000,"kind":"snapshot","b64":"cw=="}
{"t_ns":3000000000,"kind":"drop","cause":"forwarding_not_ready","b64":"YQ=="}
{"t_ns":4000000000,"kind":"resize","cols":202,"rows":48}
{"t_ns":4750000000,"kind":"snapshot","b64":"cw=="}
{"t_ns":5000000000,"kind":"resize","cols":202,"rows":40}
`
	got := PairSettleLatency(strings.NewReader(tap))
	want := []time.Duration{500 * time.Millisecond, 750 * time.Millisecond}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("PairSettleLatency = %v, want %v", got, want)
	}
}

func TestPairSettleLatency_should_HandleEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		tap  string
		want []time.Duration
	}{
		{"empty input", ``, nil},
		{"snapshot without a resize is ignored", `{"t_ns":10,"kind":"snapshot"}` + "\n", nil},
		{"aborted resize is replaced by the next one",
			`{"t_ns":1000,"kind":"resize"}` + "\n" + `{"t_ns":5000,"kind":"resize"}` + "\n" + `{"t_ns":7000,"kind":"snapshot"}` + "\n",
			[]time.Duration{2000}},
		{"clock step yields no negative sample",
			`{"t_ns":9000,"kind":"resize"}` + "\n" + `{"t_ns":4000,"kind":"snapshot"}` + "\n", nil},
		{"torn trailing line keeps earlier records",
			`{"t_ns":1000,"kind":"resize"}` + "\n" + `{"t_ns":3000,"kind":"snapshot"}` + "\n" + `{"t_ns":40`,
			[]time.Duration{2000}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := PairSettleLatency(strings.NewReader(tc.tap))
			if len(got) != len(tc.want) {
				t.Fatalf("PairSettleLatency = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("PairSettleLatency = %v, want %v", got, tc.want)
				}
			}
		})
	}
}
