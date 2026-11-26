package quota

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	quotalog "github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/tokens"
)

// The golden was recorded from the pre-move code (origin/main 41b26aa08, package
// server/services); the moved code must reproduce it byte for byte.
var updateQuotaGolden = flag.Bool("update-quota-golden", false, "rewrite testdata/quota.golden.gz")

type goldenCtrl struct {
	enabled                bool
	enableErr, disableErr  error
	enableCalls, disCalled int
}

func (c *goldenCtrl) Enable(context.Context) error {
	c.enableCalls++
	if c.enableErr != nil {
		return c.enableErr
	}
	c.enabled = true
	return nil
}

func (c *goldenCtrl) Disable() error {
	c.disCalled++
	if c.disableErr != nil {
		return c.disableErr
	}
	c.enabled = false
	return nil
}

func (c *goldenCtrl) IsEnabled() bool { return c.enabled }

type goldenCase struct {
	name         string
	cfg          config.QuotaConfig
	used         int64
	loading      bool
	nilStore     bool
	hardAgo      time.Duration // 0 = no rate-limit event
	ctrl         goldenCtrl
	paused       bool
	reason       string
	lastSet      string // "", "true", "false"
	below        int
	above        int
	overrideAgo  time.Duration
	pauseNtfAgo  time.Duration
	resumeNtfAgo time.Duration
	uncalAgo     time.Duration
	throttleLeft time.Duration
	instances    []*instance.Instance
	ticks        int
	usedPerTick  []int64
	beforeTick   func(tick int, g *QuotaGate, c *goldenCtrl)
}

func goldenCfg(budget int64) config.QuotaConfig {
	c := config.QuotaConfig{}.QuotaConfigOrDefault()
	c.Enabled = true
	c.AssumedWindowTokenBudget = budget
	return c
}

var goldenClock = regexp.MustCompile(`\d{1,2}:\d{2} [AP]M`)

func goldenLogs(buf *quotalog.SyncBuffer) string {
	var out []string
	for _, line := range strings.Split(buf.String(), "\n") {
		if !strings.Contains(line, "QuotaGate") && !strings.Contains(line, "quota") {
			continue
		}
		if i := strings.Index(line, "level="); i >= 0 {
			line = line[i:]
		}
		out = append(out, line)
	}
	return strings.Join(out, " | ")
}

func runGoldenCase(t *testing.T, w io.Writer, tc goldenCase) {
	t.Helper()
	buf := &quotalog.SyncBuffer{}
	prev := quotalog.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(buf, nil)))
	defer quotalog.SetSlogDefaultForTest(prev)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus := events.NewEventBus(64)
	ch, _ := bus.Subscribe(ctx)

	ctrl := tc.ctrl
	store := &svctest.FakeTokenStore{Results: svctest.FakeResultsWithUsage(tc.used), Loading: tc.loading}
	var reader tokens.TokenStoreReader
	if !tc.nilStore {
		reader = store
	}
	cfg := tc.cfg
	g := newGoldenGate(func() config.QuotaConfig { return cfg }, reader,
		&svctest.MockInstancePoller{Instances: tc.instances}, &ctrl, bus)

	now := time.Now()
	ago := func(d time.Duration) time.Time {
		if d == 0 {
			return time.Time{}
		}
		return now.Add(-d)
	}
	g.rateLimits.LastEventAt = ago(tc.hardAgo)
	g.state.pausedByQuota = tc.paused
	g.state.lastPauseReason = tc.reason
	if tc.lastSet != "" {
		v := tc.lastSet == "true"
		g.state.lastSetEnabled = &v
	}
	g.state.consecutiveBelow, g.state.consecutiveAbove = tc.below, tc.above
	g.state.manualOverrideAt = ago(tc.overrideAgo)
	g.state.lastPauseNotifyAt = ago(tc.pauseNtfAgo)
	g.state.lastResumeNotifyAt = ago(tc.resumeNtfAgo)
	g.state.lastUncalibratedLogAt = ago(tc.uncalAgo)
	if tc.throttleLeft != 0 {
		g.foregroundThrottleUntil = now.Add(tc.throttleLeft)
	}

	ticks := tc.ticks
	if ticks == 0 {
		ticks = 1
	}
	if len(tc.usedPerTick) > 0 {
		ticks = len(tc.usedPerTick)
	}
	fmt.Fprintf(w, "## %s\n", tc.name)
	for i := 0; i < ticks; i++ {
		if len(tc.usedPerTick) > 0 {
			store.Results = svctest.FakeResultsWithUsage(tc.usedPerTick[i])
		}
		if tc.beforeTick != nil {
			tc.beforeTick(i, g, &ctrl)
		}
		prevPauseNtf, prevResumeNtf, prevOverride := g.state.lastPauseNotifyAt, g.state.lastResumeNotifyAt, g.state.manualOverrideAt
		buf2 := len(buf.String())
		g.Reconcile(ctx)
		lastSet := "nil"
		if g.state.lastSetEnabled != nil {
			lastSet = fmt.Sprint(*g.state.lastSetEnabled)
		}
		fmt.Fprintf(w, "t%d on=%t en=%d dis=%d paused=%t isPaused=%t throttle=%t reason=%q below=%d above=%d last=%s override=%t pauseNtf=%t resumeNtf=%t est=%.4f/%d/%t status=%q\n",
			i, ctrl.enabled, ctrl.enableCalls, ctrl.disCalled, g.state.pausedByQuota, g.IsPausedByQuota(), g.ShouldThrottleForeground(),
			g.state.lastPauseReason, g.state.consecutiveBelow, g.state.consecutiveAbove, lastSet,
			!g.state.manualOverrideAt.Equal(prevOverride), !g.state.lastPauseNotifyAt.Equal(prevPauseNtf), !g.state.lastResumeNotifyAt.Equal(prevResumeNtf),
			g.state.lastEstimate.PctRemaining, g.state.lastEstimate.TokensUsed, g.state.lastEstimate.Valid, g.StatusDetail())
		if logs := goldenLogs(buf); len(buf.String()) != buf2 && logs != "" {
			fmt.Fprintf(w, "  logs %s\n", goldenClock.ReplaceAllString(logs, "CLOCK"))
			buf.Reset()
		}
		for drained := false; !drained; {
			select {
			case ev := <-ch:
				_, idErr := uuid.Parse(ev.NotificationID)
				meta, _ := json.Marshal(ev.NotificationMetadata)
				fmt.Fprintf(w, "  event %v session=%q name=%q uuid=%t type=%d prio=%d title=%q msg=%q meta=%s\n",
					ev.Type, ev.SessionID, ev.Context, idErr == nil, ev.NotificationType, ev.NotificationPriority,
					ev.NotificationTitle, goldenClock.ReplaceAllString(ev.NotificationMessage, "CLOCK"), meta)
			default:
				drained = true
			}
		}
	}
}

func goldenGateCases() []goldenCase {
	var cases []goldenCase
	// Core cross product: budget x usage around both thresholds x hard signal x controller x provenance.
	for _, budget := range []int64{0, 1000} {
		for _, used := range []int64{0, 649, 650, 651, 799, 800, 801, 2000} {
			for _, hard := range []time.Duration{0, time.Minute, 31 * time.Minute} {
				for _, on := range []bool{true, false} {
					for _, paused := range []bool{true, false} {
						for _, lastSet := range []string{"", "true", "false"} {
							cases = append(cases, goldenCase{
								name: fmt.Sprintf("core budget=%d used=%d hard=%s on=%t paused=%t last=%q", budget, used, hard, on, paused, lastSet),
								cfg:  goldenCfg(budget), used: used, hardAgo: hard, ctrl: goldenCtrl{enabled: on},
								paused: paused, reason: "soft_threshold", lastSet: lastSet, below: 1, above: 2,
							})
						}
					}
				}
			}
		}
	}
	// Gate flag off, token store loading or missing, and controller errors.
	for _, enabled := range []bool{true, false} {
		for _, store := range []string{"ok", "loading", "nil"} {
			for _, on := range []bool{true, false} {
				for _, paused := range []bool{true, false} {
					for _, errs := range []string{"none", "enable", "disable"} {
						for _, used := range []int64{0, 950} {
							for _, hard := range []time.Duration{0, time.Minute} {
								cfg := goldenCfg(1000)
								cfg.Enabled = enabled
								c := goldenCtrl{enabled: on}
								if errs == "enable" {
									c.enableErr = errors.New("enable boom")
								}
								if errs == "disable" {
									c.disableErr = errors.New("disable boom")
								}
								cases = append(cases, goldenCase{
									name: fmt.Sprintf("flags enabled=%t store=%s on=%t paused=%t err=%s used=%d hard=%s", enabled, store, on, paused, errs, used, hard),
									cfg:  cfg, used: used, loading: store == "loading", nilStore: store == "nil", hardAgo: hard,
									ctrl: c, paused: paused, reason: "hard_override", below: 1, above: 2, ticks: 2,
								})
							}
						}
					}
				}
			}
		}
	}
	// Foreground throttle: which sessions count, and existing deadlines.
	sessions := map[string][]*instance.Instance{
		"none":          nil,
		"nil-entry":     {nil},
		"active":        {{Category: "", Status: domain.Active}},
		"backlog":       {{Category: instance.CategoryBacklog, Status: domain.Active}},
		"paused":        {{Category: "", Status: domain.Paused}},
		"backlog+human": {{Category: instance.CategoryBacklog, Status: domain.Active}, nil, {Category: "work", Status: domain.Active}},
	}
	for _, name := range []string{"none", "nil-entry", "active", "backlog", "paused", "backlog+human"} {
		for _, left := range []time.Duration{0, time.Hour, -time.Hour} {
			for _, on := range []bool{true, false} {
				cfg := goldenCfg(0)
				cases = append(cases, goldenCase{
					name: fmt.Sprintf("foreground %s left=%s on=%t", name, left, on),
					cfg:  cfg, instances: sessions[name], throttleLeft: left, ctrl: goldenCtrl{enabled: on},
					uncalAgo: time.Minute, ticks: 2,
				})
			}
		}
	}
	// Notification cooldown and manual-override grace.
	for _, override := range []time.Duration{0, 5 * time.Minute, 15 * time.Minute} {
		for _, last := range []time.Duration{0, 2 * time.Minute, 6 * time.Minute} {
			cfg := goldenCfg(1000)
			cfg.ConsecutiveTicksToPause, cfg.ConsecutiveTicksToResume = 1, 1
			cases = append(cases,
				goldenCase{name: fmt.Sprintf("notify soft override=%s last=%s", override, last), cfg: cfg, used: 900,
					ctrl: goldenCtrl{enabled: true}, overrideAgo: override, pauseNtfAgo: last},
				goldenCase{name: fmt.Sprintf("notify hard override=%s last=%s", override, last), cfg: cfg, hardAgo: time.Minute,
					ctrl: goldenCtrl{enabled: true}, overrideAgo: override, pauseNtfAgo: last},
				goldenCase{name: fmt.Sprintf("notify resume override=%s last=%s", override, last), cfg: cfg,
					ctrl: goldenCtrl{enabled: false}, paused: true, lastSet: "false", overrideAgo: override, resumeNtfAgo: last},
			)
		}
	}
	// Uncalibrated usage log cadence.
	for _, uncal := range []time.Duration{0, 30 * time.Minute, 2 * time.Hour} {
		cases = append(cases, goldenCase{name: fmt.Sprintf("uncalibrated lastLog=%s", uncal), cfg: goldenCfg(0), used: 1234,
			ctrl: goldenCtrl{enabled: true}, uncalAgo: uncal, ticks: 2})
	}
	// Hysteresis and provenance sequences.
	manual := func(at map[int]bool) func(int, *QuotaGate, *goldenCtrl) {
		return func(tick int, _ *QuotaGate, c *goldenCtrl) {
			if v, ok := at[tick]; ok {
				c.enabled = v
			}
		}
	}
	cases = append(cases,
		goldenCase{name: "seq pause then resume", cfg: goldenCfg(1000), ctrl: goldenCtrl{enabled: true},
			usedPerTick: []int64{900, 100, 900, 900, 900, 100, 100, 700, 100, 100, 100, 100}},
		goldenCase{name: "seq manual re-enable while critical", cfg: goldenCfg(1000), ctrl: goldenCtrl{enabled: true},
			usedPerTick: []int64{900, 900, 900, 900, 900, 900}, beforeTick: manual(map[int]bool{3: true})},
		goldenCase{name: "seq manual disable never resumed", cfg: goldenCfg(1000), ctrl: goldenCtrl{enabled: true},
			usedPerTick: []int64{0, 0, 0, 0, 0}, beforeTick: manual(map[int]bool{1: false})},
		goldenCase{name: "seq hard clears then resumes", cfg: goldenCfg(1000), ctrl: goldenCtrl{enabled: true}, hardAgo: time.Minute,
			ticks: 7, beforeTick: func(tick int, g *QuotaGate, _ *goldenCtrl) {
				if tick == 3 {
					g.rateLimits.LastEventAt = time.Now().Add(-time.Hour)
				}
			}},
	)
	return cases
}

func goldenHeadroom(w io.Writer) {
	now := time.Date(2026, 1, 2, 15, 0, 0, 0, time.UTC)
	turn := func(at time.Duration, n int64) tokens.TurnStats {
		return tokens.TurnStats{Timestamp: now.Add(at), Input: n, Output: 2 * n, CacheCreation: 3 * n, CacheRead: 4 * n}
	}
	sets := map[string][]*tokens.ParseResult{
		"none": nil,
		"nil":  {nil},
		"edges": {{TurnTimeline: []tokens.TurnStats{turn(-5*time.Hour-time.Nanosecond, 1), turn(-5*time.Hour, 10),
			turn(-time.Hour, 100), turn(0, 1000), turn(time.Nanosecond, 10000)}}},
		"two": {{TurnTimeline: []tokens.TurnStats{turn(-time.Minute, 7)}}, nil, {TurnTimeline: []tokens.TurnStats{turn(-time.Minute, 11)}}},
	}
	for _, name := range []string{"none", "nil", "edges", "two"} {
		for _, budget := range []int64{-1, 0, 1, 180, 1000, 11100, 5_000_000} {
			for _, loading := range []bool{false, true} {
				e := computeHeadroom(sets[name], budget, loading, now)
				fmt.Fprintf(w, "headroom %s budget=%d loading=%t start=%s end=%s used=%d assumed=%d pct=%.6f valid=%t\n",
					name, budget, loading, e.WindowStart.Format(time.RFC3339), e.WindowEnd.Format(time.RFC3339),
					e.TokensUsed, e.AssumedBudget, e.PctRemaining, e.Valid)
			}
		}
	}
}

func goldenLogAttrs(w io.Writer) {
	usd, zero := 12.5, 0.0
	for i, c := range []config.QuotaConfig{
		{}, goldenCfg(0), goldenCfg(5), {Enabled: true, PauseBelowHeadroomPct: 30, ResumeMarginPct: 5, AssumedWindowTokenBudget: 9,
			ConsecutiveTicksToPause: 4, ConsecutiveTicksToResume: 5, RateLimitWindowMinutes: 6, ManualOverrideGraceMinutes: 7,
			ForegroundThrottleDelaySeconds: 8, DefaultCostBudgetThresholdUSD: &usd}, {DefaultCostBudgetThresholdUSD: &zero},
	} {
		fmt.Fprintf(w, "attrs %d %v\n", i, quotaLogAttrs(c))
	}
}

func goldenRPC(t *testing.T, w io.Writer) {
	t.Helper()
	svctest.WithIsolatedStateDir(t)
	mux := http.NewServeMux()
	mux.Handle(sessionv1connect.NewQuotaServiceHandler(NewQuotaSettingsService()))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	dir, err := config.GetConfigDir()
	require.NoError(t, err)
	path := filepath.Join(dir, config.ConfigFileName)

	call := func(method, body string) {
		resp, err := http.Post(srv.URL+"/session.v1.QuotaService/"+method, "application/json", strings.NewReader(body))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		raw, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		var compact bytes.Buffer
		require.NoError(t, json.Compact(&compact, raw))
		fmt.Fprintf(w, "rpc %s %s -> %d %s\n", method, body, resp.StatusCode,
			strings.ReplaceAll(compact.String(), dir, "STATEDIR"))
		var file map[string]json.RawMessage
		if data, err := os.ReadFile(path); err == nil && json.Unmarshal(data, &file) == nil {
			var q bytes.Buffer
			require.NoError(t, json.Compact(&q, file["quota"]))
			_, stamped := file["quota_configured_at"]
			fmt.Fprintf(w, "  file quota=%s stamped=%t\n", q.String(), stamped)
		}
	}
	get := func() { call("GetQuotaConfig", `{}`) }
	update := func(fields string) { call("UpdateQuotaConfig", `{"settings":{`+fields+`}}`) }

	get()
	call("UpdateQuotaConfig", `{}`)
	update(`"enabled":true`)
	get()
	update(`"assumedWindowTokenBudget":"5000000","pauseBelowHeadroomPct":30,"resumeMarginPct":10,"consecutiveTicksToPause":4,"consecutiveTicksToResume":5,"rateLimitWindowMinutes":6,"manualOverrideGraceMinutes":7,"foregroundThrottleDelaySeconds":8,"defaultCostBudgetThresholdUsd":2.5`)
	update(``)
	update(`"defaultCostBudgetThresholdUsd":0`)
	update(`"enabled":false`)
	for _, f := range []string{"pauseBelowHeadroomPct", "resumeMarginPct"} {
		for _, v := range []string{"0", "-1", "0.5", "100", "100.5", `"NaN"`, `"Infinity"`} {
			update(fmt.Sprintf("%q:%s", f, v))
		}
	}
	update(`"pauseBelowHeadroomPct":60,"resumeMarginPct":40`)
	update(`"pauseBelowHeadroomPct":60,"resumeMarginPct":40.001`)
	for _, f := range []struct {
		name string
		max  int
	}{
		{"consecutiveTicksToPause", maxQuotaTicks}, {"consecutiveTicksToResume", maxQuotaTicks},
		{"rateLimitWindowMinutes", maxQuotaMinutes}, {"manualOverrideGraceMinutes", maxQuotaMinutes},
		{"foregroundThrottleDelaySeconds", maxQuotaSeconds},
	} {
		for _, v := range []int{-1, 0, 1, f.max, f.max + 1, math.MaxInt32} {
			update(fmt.Sprintf("%q:%d", f.name, v))
		}
	}
	for _, v := range []string{`"-1"`, `"0"`, `"1"`} {
		update(`"assumedWindowTokenBudget":` + v)
	}
	for _, v := range []string{"-0.01", `"NaN"`, `"Infinity"`, `"-Infinity"`} {
		update(`"defaultCostBudgetThresholdUsd":` + v)
	}
	update(`"enabled":true,"assumedWindowTokenBudget":"0"`)
	get()
	// Unreadable config: refused, reported as internal.
	require.NoError(t, os.Remove(path))
	require.NoError(t, os.Mkdir(path, 0o700))
	update(`"enabled":true`)
}

func TestQuotaGolden_should_MatchPreMoveBehaviour_When_Replayed(t *testing.T) {
	var out bytes.Buffer
	goldenHeadroom(&out)
	goldenLogAttrs(&out)
	for _, tc := range goldenGateCases() {
		runGoldenCase(t, &out, tc)
	}
	goldenRPC(t, &out)

	golden := filepath.Join("testdata", "quota.golden.gz")
	if *updateQuotaGolden {
		writeGoldenGzip(t, golden, out.Bytes())
		return
	}
	want := readGoldenGzip(t, golden)
	gotLines, wantLines := strings.Split(out.String(), "\n"), strings.Split(string(want), "\n")
	for i := range gotLines {
		if i >= len(wantLines) || gotLines[i] != wantLines[i] {
			wantLine := "<eof>"
			if i < len(wantLines) {
				wantLine = wantLines[i]
			}
			t.Fatalf("golden differs at line %d\n got: %s\nwant: %s", i+1, gotLines[i], wantLine)
		}
	}
	require.Len(t, gotLines, len(wantLines))
}

func writeGoldenGzip(t *testing.T, path string, data []byte) {
	t.Helper()
	var packed bytes.Buffer
	zw, err := gzip.NewWriterLevel(&packed, gzip.BestCompression)
	require.NoError(t, err)
	_, err = zw.Write(data)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	require.NoError(t, os.WriteFile(path, packed.Bytes(), 0o600))
}

func readGoldenGzip(t *testing.T, path string) []byte {
	t.Helper()
	packed, err := os.ReadFile(path)
	require.NoError(t, err)
	zr, err := gzip.NewReader(bytes.NewReader(packed))
	require.NoError(t, err)
	data, err := io.ReadAll(zr)
	require.NoError(t, err)
	return data
}
