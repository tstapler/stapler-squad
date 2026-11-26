package streamhubrollout

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/instance"
)

var updateStreamHubGolden = flag.Bool("update-streamhubrollout-golden", false, "rewrite testdata/streamhubrollout.golden")

type streamHubGolden struct {
	t   *testing.T
	dir string
	b   strings.Builder
}

func (g *streamHubGolden) line(format string, args ...any) {
	fmt.Fprintf(&g.b, format+"\n", args...)
}

// result records the response wire bytes (rehearsal time replaced, map-ordered overrides sorted) and the persisted config.
func (g *streamHubGolden) result(label string, resp *connect.Response[sessionv1.StreamHubRolloutStatus], err error) {
	if err != nil {
		var ce *connect.Error
		code := "none"
		if errors.As(err, &ce) {
			code = ce.Code().String()
		}
		g.line("%s: code=%s err=%q", label, code, strings.ReplaceAll(err.Error(), g.dir, "<dir>"))
	} else {
		m := proto.Clone(resp.Msg).(*sessionv1.StreamHubRolloutStatus)
		if m.RollbackRehearsalCompletedAt != nil {
			require.WithinDuration(g.t, time.Now(), m.RollbackRehearsalCompletedAt.AsTime(), time.Minute)
			m.RollbackRehearsalCompletedAt = timestamppb.New(time.Unix(1, 0))
		}
		sort.Slice(m.SessionOverrides, func(i, j int) bool {
			return m.SessionOverrides[i].SessionName < m.SessionOverrides[j].SessionName
		})
		raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(m)
		require.NoError(g.t, err)
		g.line("%s: bytes=%s", label, hex.EncodeToString(raw))
	}
	cfg := config.LoadConfig()
	keys := make([]string, 0, len(cfg.StreamHubSessionOverrides))
	for k, v := range cfg.StreamHubSessionOverrides {
		keys = append(keys, fmt.Sprintf("%s=%t", k, v))
	}
	sort.Strings(keys)
	v, ok := cfg.GetStreamHubGlobalOverride()
	g.line("  persisted overrides=%v global=%t/%t rehearsed=%t", keys, v, ok, cfg.RollbackRehearsalCompletedAt != nil)
}

// The golden was recorded from the pre-move code in server/services at 385c81549.
func TestStreamHubRolloutGolden(t *testing.T) {
	g := &streamHubGolden{t: t, dir: svctest.WithIsolatedStateDir(t)}
	t.Setenv("STAPLER_SQUAD_USE_STREAM_HUB", "true")
	ctx := context.Background()

	for _, c := range [][2]string{{"a", ""}, {"my session.v2:x", ""}, {"a", "custom_"}, {"", ""}, {"staplersquad_a", ""}} {
		g.line("key(%q,%q)=%q", c[0], c[1], StreamHubSessionKey(c[0], c[1]))
	}
	custom := &instance.Instance{Title: "custom one", TmuxPrefix: "pfx_"}
	plain := &instance.Instance{Title: "plain"}
	g.line("path(custom)=%q path(plain)=%q", TmuxSessionNameForStreamPath(custom), TmuxSessionNameForStreamPath(plain))

	finder := func(title string) *instance.Instance {
		switch title {
		case "custom one":
			return custom
		case "plain":
			return plain
		}
		return nil
	}
	yes, no := true, false
	for _, wired := range []bool{false, true} {
		var s *StreamHubRolloutService
		if wired {
			s = NewStreamHubRolloutService(finder)
		} else {
			s = NewStreamHubRolloutService(nil)
		}
		g.line("== finder wired=%t", wired)
		get := func(label string) {
			resp, err := s.GetStreamHubRolloutStatus(ctx, connect.NewRequest(&sessionv1.GetStreamHubRolloutStatusRequest{}))
			g.result(label, resp, err)
		}
		session := func(name string, force *bool) {
			resp, err := s.SetStreamHubSessionOverride(ctx, connect.NewRequest(&sessionv1.SetStreamHubSessionOverrideRequest{SessionName: name, ForceHub: force}))
			g.result(fmt.Sprintf("session %q %s", name, fmtBoolPtr(force)), resp, err)
		}
		global := func(force *bool) {
			resp, err := s.SetStreamHubGlobalOverride(ctx, connect.NewRequest(&sessionv1.SetStreamHubGlobalOverrideRequest{ForceHub: force}))
			g.result("global "+fmtBoolPtr(force), resp, err)
		}
		get("get")
		for _, name := range []string{"custom one", "plain", "unknown", ""} {
			session(name, &yes)
		}
		session("custom one", &no)
		for _, name := range []string{"custom one", "plain", "unknown", "", "never set"} {
			session(name, nil)
		}
		global(&yes)
		global(&no)
		get("get with global")
		global(nil)
		resp, err := s.CompleteStreamHubRollbackRehearsal(ctx, connect.NewRequest(&sessionv1.CompleteStreamHubRollbackRehearsalRequest{}))
		g.result("rehearsal", resp, err)
		get("get after rehearsal")
	}

	// A config.json that is a directory makes every save fail.
	g.dir = svctest.WithIsolatedStateDir(t)
	configDir, err := config.GetConfigDir()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(configDir, config.ConfigFileName, "block"), 0o750))
	s := NewStreamHubRolloutService(nil)
	g.line("== unwritable config")
	get, err := s.GetStreamHubRolloutStatus(ctx, connect.NewRequest(&sessionv1.GetStreamHubRolloutStatusRequest{}))
	g.result("get", get, err)
	r1, err := s.CompleteStreamHubRollbackRehearsal(ctx, connect.NewRequest(&sessionv1.CompleteStreamHubRollbackRehearsalRequest{}))
	g.result("rehearsal", r1, maskTemp(err))
	r2, err := s.SetStreamHubSessionOverride(ctx, connect.NewRequest(&sessionv1.SetStreamHubSessionOverrideRequest{SessionName: "x", ForceHub: &yes}))
	g.result("session set", r2, maskTemp(err))
	r3, err := s.SetStreamHubSessionOverride(ctx, connect.NewRequest(&sessionv1.SetStreamHubSessionOverrideRequest{SessionName: "x"}))
	g.result("session clear", r3, maskTemp(err))
	r4, err := s.SetStreamHubGlobalOverride(ctx, connect.NewRequest(&sessionv1.SetStreamHubGlobalOverrideRequest{ForceHub: &yes}))
	g.result("global set", r4, maskTemp(err))
	r5, err := s.SetStreamHubGlobalOverride(ctx, connect.NewRequest(&sessionv1.SetStreamHubGlobalOverrideRequest{}))
	g.result("global clear", r5, maskTemp(err))

	path := filepath.Join("testdata", "streamhubrollout.golden")
	if *updateStreamHubGolden {
		require.NoError(t, os.MkdirAll("testdata", 0o750))
		require.NoError(t, os.WriteFile(path, []byte(g.b.String()), 0o600))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), g.b.String())
}

func fmtBoolPtr(b *bool) string {
	if b == nil {
		return "nil"
	}
	return fmt.Sprint(*b)
}

// maskTemp replaces the random suffix of the atomic-write temp file, keeping the connect code.
func maskTemp(err error) error {
	var ce *connect.Error
	if !errors.As(err, &ce) {
		return err
	}
	return connect.NewError(ce.Code(), errors.New(goldenTempName.ReplaceAllString(ce.Message(), ".tmp-N")))
}

var goldenTempName = regexp.MustCompile(`\.[0-9]+\.tmp`)
