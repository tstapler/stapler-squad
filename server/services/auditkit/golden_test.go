package auditkit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services/fsops/memfs"
)

// testdata/audit_golden.txt was recorded from the pre-move implementation in
// server/services (hidden_reply_audit.go at a73494b4a): the audit lines, boot_seq
// counter, rotation layout, fallback WARN records and error texts, byte for byte
// apart from the random boot id.
func TestAuditSink_ShouldReproduceThePreMoveRecording_WhenDrivenByTheSameScript(t *testing.T) {
	t.Parallel()
	want, err := os.ReadFile("testdata/audit_golden.txt")
	require.NoError(t, err)

	got := recordGolden(t)

	require.Equal(t, string(want), got)
}

func goldenLines() []AuditLine {
	tr, fa, n := true, false, 3
	return []AuditLine{
		{Kind: AuditKindFlagChg, Phase: AuditPhaseRequest, ChangeID: "c1", Flag: "f", Scope: "global", Mutation: "set", Previous: &tr, New: &fa, Seq: 7, Listener: "local", PeerAddr: "127.0.0.1:1", Host: "h", Origin: "o", UserAgent: "ua", AuthMode: "none"},
		{Kind: AuditKindFlagChg, Phase: AuditPhaseResult, ChangeID: "c1", Outcome: FlagOutcomeApplied},
		{Kind: AuditKindReply, Phase: AuditPhaseRequest, SessionUUID: "su", QuestionID: "q", ReplyID: "r", OptionIndex: 2, OptionLabel: "Yes", QuestionToken: "tok", Stage: "send"},
		{Kind: AuditKindBacklogSteer, Phase: AuditPhaseResult, SessionUUID: "su", SessionTitle: "T", ItemID: "i", MessageLen: 5, MessageSHA256: "abc", MessagePreview: "hello", PeerLoopback: &tr, Proxied: &fa},
		{Kind: AuditKindGuardBypass, Phase: AuditPhaseRequest, Outcome: FlagOutcomeRolledBack},
		{Kind: AuditKindPrune, Phase: AuditPhaseRequest, MatchedCount: &n, IncludeUnreadActionable: &tr, Counts: map[string]int{"b": 2, "a": 1}},
		{Kind: AuditKindNotificationAction, Phase: AuditPhaseResult, Channel: "ch", CallerKind: "ui", ActionKind: "ack", TargetKey: "tk", Detail: "d", Outcome: FlagOutcomePersistFailed},
	}
}

func recordGolden(t *testing.T) string {
	t.Helper()
	var out strings.Builder
	norm := func(s string) string { return regexp.MustCompile(`[0-9a-f]{16}`).ReplaceAllString(s, "<boot>") }
	now := time.Date(2026, 1, 2, 3, 4, 5, 123456789, time.UTC)
	clock := WithAuditClock(func() time.Time { return now })
	lines := goldenLines()

	mfs := memfs.New()
	recs := &logCapture{}
	var modes []string
	s := NewAuditSink(cfgDir, WithAuditFS(mfs), clock, WithAuditLogger(slog.New(recs)),
		WithAuditDegradedCounter(func(m string) { modes = append(modes, m) }))
	for i, l := range lines {
		fmt.Fprintf(&out, "append %d err=%v\n", i, s.Append(l))
	}
	raw, _ := mfs.Get(testAuditFile)
	out.WriteString(norm(string(raw)))
	bs, _ := mfs.Get("/cfg/audit/boot_seq")
	fmt.Fprintf(&out, "boot_seq=%s\nmode=%o\nclosed-enqueue\n", bs, mfs.Modes[testAuditFile])
	s.Close()
	s.Enqueue(lines[3])
	s.Enqueue(lines[0])
	fmt.Fprintf(&out, "modes=%v\n", modes)
	for _, r := range recs.recs {
		out.WriteString(norm(r) + "\n")
	}

	mfs2 := memfs.New()
	s2 := NewAuditSink(cfgDir, WithAuditFS(mfs2), clock)
	s2.Enqueue(lines[0])
	s2.Enqueue(lines[1])
	s2.Close()
	raw, _ = mfs2.Get(testAuditFile)
	out.WriteString("queued:\n" + norm(string(raw)))

	mfs3 := memfs.New()
	mfs3.Put("/cfg/audit/boot_seq", []byte("garbage"))
	s3 := NewAuditSink(cfgDir, WithAuditFS(mfs3), clock, WithAuditLogger(slog.New(&logCapture{})))
	fmt.Fprintf(&out, "corrupt err=%v\n", s3.Append(lines[1]))
	bs, _ = mfs3.Get("/cfg/audit/boot_seq")
	fmt.Fprintf(&out, "boot_seq=%s\n", bs)
	s3.Close()

	mfs4 := memfs.New()
	mfs4.FailMkdir = errors.New("mkdir boom")
	s4 := NewAuditSink(cfgDir, WithAuditFS(mfs4))
	fmt.Fprintf(&out, "mkdir err=%v is=%v\n", s4.Append(lines[0]), errors.Is(s4.Append(lines[0]), ErrAuditFailed))
	s4.Close()

	s5 := NewAuditSink(func() (string, error) { return "", errors.New("no dir") }, WithAuditFS(memfs.New()))
	fmt.Fprintf(&out, "dirfn err=%v\n", s5.Append(lines[0]))
	s5.Close()

	mfs6 := memfs.New()
	mfs6.SetFailWrite(errDiskFull)
	s6 := NewAuditSink(cfgDir, WithAuditFS(mfs6))
	fmt.Fprintf(&out, "write err=%v\n", s6.Append(lines[0]))
	s6.Close()

	timer := make(chan time.Time, 1)
	timer <- now
	mfs7 := memfs.New()
	mfs7.SyncStall = make(chan struct{})
	mfs7.SyncEntered = make(chan struct{}, 1)
	s7 := NewAuditSink(cfgDir, WithAuditFS(mfs7), WithAuditAfter(func(time.Duration) <-chan time.Time { return timer }))
	err := s7.AppendBounded(context.Background(), lines[0])
	fmt.Fprintf(&out, "bounded err=%v is=%v\n", err, errors.Is(err, ErrAuditTimeout))
	close(mfs7.SyncStall)
	s7.Close()

	mfs9 := memfs.New()
	s9 := NewAuditSink(cfgDir, WithAuditFS(mfs9), clock)
	big := AuditLine{Kind: "flag_change", UserAgent: strings.Repeat("x", 2<<20)}
	for i := 0; i < 12; i++ {
		require.NoError(t, s9.Append(big))
	}
	for _, name := range mfs9.Names("/cfg/audit/") {
		b, _ := mfs9.Get(name)
		fmt.Fprintf(&out, "file %s size=%d\n", name, len(b))
	}
	s9.Close()
	return out.String()
}
