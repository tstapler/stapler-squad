package streamhub

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// steppedTap returns a tap writing to buf whose clock advances 10ms per record.
func steppedTap(buf *bytes.Buffer) *CaptureTap {
	tap := newCaptureTapWriter(buf)
	tick := time.Unix(1000, 0)
	tap.now = func() time.Time { tick = tick.Add(10 * time.Millisecond); return tick }
	return tap
}

func TestCaptureTap_should_WriteNothingAndAllocateNothing_When_EnvVarUnset(t *testing.T) {
	t.Setenv(CaptureTapDirEnv, "")
	if tap := CaptureTapFor("corp-compute-nop-pr-534"); tap != nil {
		t.Fatalf("CaptureTapFor with env unset = %v, want nil", tap)
	}

	hub := NewStreamHub("corp-compute-nop-pr-534", nil, WithCaptureTap(CaptureTapFor("corp-compute-nop-pr-534")))
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
	for _, want := range []string{`"kind":"drop"`, `"cause":"resize_settling"`, `"b64":"G1syS2Zvbw=="`} {
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

func TestCaptureTap_should_WriteSanitizedPerSessionFile_When_EnvVarSet(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(CaptureTapDirEnv, dir)

	tap := CaptureTapFor("a/b c:1")
	if tap == nil || tap != CaptureTapFor("a/b c:1") {
		t.Fatal("expected a shared non-nil tap per session")
	}
	tap.Record(TapOutput, "", []byte("x"))

	data, err := os.ReadFile(filepath.Join(dir, "a_b_c_1.jsonl"))
	if err != nil || !strings.Contains(string(data), `"kind":"output"`) {
		t.Fatalf("file = %q, err = %v", data, err)
	}
}

func TestPairSettleLatency_should_PairFirstDropWithNextSnapshot_When_SyntheticTapRead(t *testing.T) {
	const tap = `{"t_ns":1000000000,"kind":"output","b64":"YQ=="}
{"t_ns":2000000000,"kind":"drop","cause":"resize_settling","b64":"YQ=="}
{"t_ns":2100000000,"kind":"drop","cause":"resize_settling","b64":"Yg=="}
{"t_ns":2500000000,"kind":"snapshot","b64":"cw=="}
{"t_ns":3000000000,"kind":"drop","cause":"forwarding_not_ready","b64":"YQ=="}
{"t_ns":4000000000,"kind":"drop","cause":"resize_settling","b64":"YQ=="}
{"t_ns":4750000000,"kind":"snapshot","b64":"cw=="}
{"t_ns":5000000000,"kind":"drop","cause":"resize_settling","b64":"YQ=="}
`
	got := PairSettleLatency(strings.NewReader(tap))
	want := []time.Duration{500 * time.Millisecond, 750 * time.Millisecond}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("PairSettleLatency = %v, want %v", got, want)
	}
}
