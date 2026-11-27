package goalrunner

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeClaude writes an executable that prints stdout verbatim and records its argv.
func fakeClaude(t *testing.T, stdout string) (bin, argvFile string) {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "out.ndjson")
	argvFile = filepath.Join(dir, "argv")
	if err := os.WriteFile(out, []byte(stdout), 0o600); err != nil {
		t.Fatal(err)
	}
	bin = filepath.Join(dir, "claude")
	script := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\"; done > " + argvFile + "\ncat " + out + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return bin, argvFile
}

func TestRun_should_ParseEndReasonAndCost_When_StreamEnds(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		stdout string
		want   EndReason
		cost   float64
		turns  int
	}{
		{"goal_met", `{"type":"result","end_reason":"goal_met","cost_usd":0.42,"num_turns":3}` + "\n", EndGoalMet, 0.42, 3},
		{"turn_limit", `{"type":"result","end_reason":"turn_limit","cost_usd":1.1,"num_turns":20}` + "\n", EndTurnLimit, 1.1, 20},
		{"impossible", `{"type":"result","end_reason":"impossible","cost_usd":0.1,"num_turns":2}` + "\n", EndImpossible, 0.1, 2},
		{"error_flag", `{"type":"result","is_error":true,"total_cost_usd":0.2,"num_turns":1}` + "\n", EndError, 0.2, 1},
		{"max_turns_subtype", `{"type":"result","subtype":"error_max_turns","total_cost_usd":0.3,"num_turns":5}` + "\n", EndTurnLimit, 0.3, 5},
		{"malformed_skipped", "not json\n" + `{"type":"result","end_reason":"goal_met","cost_usd":0.5,"num_turns":1}` + "\n", EndGoalMet, 0.5, 1},
		{"real_result_shape", `{"type":"result","subtype":"success","is_error":false,"terminal_reason":"completed","num_turns":4,"total_cost_usd":0.014469425}` + "\n", EndGoalMet, 0.014469425, 4},
		{"success_at_turn_bound", `{"type":"result","subtype":"success","is_error":false,"num_turns":20,"total_cost_usd":0.2}` + "\n", EndTurnLimit, 0.2, 20},
		{"no_result", `{"type":"system"}` + "\n", EndError, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bin, _ := fakeClaude(t, tc.stdout)
			got := Run(context.Background(), Config{Binary: bin, Criteria: "make ci exits 0", Logger: slog.New(slog.DiscardHandler)})
			if got.EndReason != tc.want || got.CostUSD != tc.cost || got.Turns != tc.turns {
				t.Fatalf("got %+v, want reason=%s cost=%v turns=%d", got, tc.want, tc.cost, tc.turns)
			}
		})
	}
}

func TestRun_should_LaunchDirectSubprocessWithGoalArgs_When_Configured(t *testing.T) {
	t.Parallel()
	bin, argvFile := fakeClaude(t, `{"type":"result","end_reason":"goal_met","cost_usd":0.01,"num_turns":1}`+"\n")
	Run(context.Background(), Config{Binary: bin, Criteria: "docs build passes", MaxTurns: 7, SettingsPath: "/tmp/s.json", Logger: slog.New(slog.DiscardHandler)})
	raw, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-p", "/goal docs build passes or stop after 7 turns", "--output-format", "stream-json", "--verbose", "--settings", "/tmp/s.json"}
	if got := strings.Split(strings.TrimSpace(string(raw)), "\n"); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("argv = %q, want %q", got, want)
	}
}

func TestRun_should_KillAndReportCostExceeded_When_CeilingCrossed(t *testing.T) {
	t.Parallel()
	// Unknown model is priced at the worst-case rate; 10M output tokens is far over $2.
	assistant := `{"type":"assistant","message":{"id":"m1","model":"unknown-model","usage":{"output_tokens":10000000}}}` + "\n"
	// A result line follows to prove the ceiling, not the result, decided the outcome.
	bin, _ := fakeClaude(t, assistant+`{"type":"result","end_reason":"goal_met","cost_usd":0.01,"num_turns":1}`+"\n")
	got := Run(context.Background(), Config{Binary: bin, Criteria: "x", Logger: slog.New(slog.DiscardHandler)})
	if got.EndReason != EndCostExceeded {
		t.Fatalf("got %+v, want cost_exceeded", got)
	}
	if got.CostUSD <= DefaultCostCeilingUSD {
		t.Fatalf("estimated cost %v should exceed ceiling", got.CostUSD)
	}
}

func TestRun_should_LogHeartbeat_When_TickerFires(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	// Hold the process open until the heartbeat has been observed.
	gate := filepath.Join(dir, "gate")
	script := "#!/bin/sh\nwhile [ ! -e " + gate + " ]; do sleep 0.01; done\n" +
		`echo '{"type":"result","end_reason":"goal_met","cost_usd":0.01,"num_turns":1}'` + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	var buf syncBuffer
	tickCh := make(chan time.Time)
	cfg := Config{
		Binary: bin, Criteria: "x", Logger: slog.New(slog.NewTextHandler(&buf, nil)),
		NewTicker: func(time.Duration) (<-chan time.Time, func()) { return tickCh, func() {} },
	}
	res := make(chan Outcome, 1)
	go func() { res <- Run(context.Background(), cfg) }()
	for range 2 { // unbuffered: the second receive guarantees the first heartbeat was logged
		select {
		case tickCh <- time.Now():
		case got := <-res:
			t.Fatalf("Run returned before heartbeat: %+v", got)
		case <-time.After(10 * time.Second):
			t.Fatal("heartbeat tick not received")
		}
	}
	if err := os.WriteFile(gate, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := <-res; got.EndReason != EndGoalMet {
		t.Fatalf("got %+v", got)
	}
	if !strings.Contains(buf.String(), "goal run heartbeat") {
		t.Fatalf("no heartbeat in log: %q", buf.String())
	}
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *syncBuffer) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func TestRun_should_KillBlockedProcess_When_CeilingCrossed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	line := `{"type":"assistant","message":{"id":"m1","model":"unknown-model","usage":{"output_tokens":10000000}}}`
	script := "#!/bin/sh\necho '" + line + "'\nexec sleep 30\n"
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	got := Run(context.Background(), Config{Binary: bin, Criteria: "x", Logger: slog.New(slog.DiscardHandler)})
	if got.EndReason != EndCostExceeded || time.Since(start) > 15*time.Second {
		t.Fatalf("got %+v after %v; the blocked process was not killed", got, time.Since(start))
	}
}

func TestRun_should_FailFast_When_LineExceedsScannerBuffer(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	script := "#!/bin/sh\nhead -c 5000000 /dev/zero | tr '\\0' 'a'\nexec sleep 30\n"
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	got := Run(context.Background(), Config{Binary: bin, Criteria: "x", Logger: slog.New(slog.DiscardHandler)})
	if got.EndReason != EndError || time.Since(start) > 15*time.Second {
		t.Fatalf("got %+v after %v", got, time.Since(start))
	}
}

func TestRun_should_ReportError_When_ProcessExitsNonZeroAfterResult(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	script := "#!/bin/sh\necho '{\"type\":\"result\",\"subtype\":\"success\",\"num_turns\":1,\"total_cost_usd\":0.1}'\nexit 3\n"
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	got := Run(context.Background(), Config{Binary: bin, Criteria: "x", Logger: slog.New(slog.DiscardHandler)})
	if got.EndReason != EndError || got.Err == nil || got.CostUSD != 0.1 {
		t.Fatalf("got %+v", got)
	}
}

func TestRun_should_ReportStartError_When_BinaryMissing(t *testing.T) {
	t.Parallel()
	got := Run(context.Background(), Config{Binary: filepath.Join(t.TempDir(), "nope"), Criteria: "x"})
	if got.EndReason != EndError || got.Err == nil {
		t.Fatalf("got %+v", got)
	}
}

func TestRun_should_AttachStderrTail_When_NoResultLine(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho 'auth failed' >&2\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	got := Run(context.Background(), Config{Binary: bin, Criteria: "x"})
	if got.EndReason != EndError || got.Err == nil || !strings.Contains(got.Err.Error(), "auth failed") {
		t.Fatalf("stderr tail missing from error: %+v", got)
	}
}
