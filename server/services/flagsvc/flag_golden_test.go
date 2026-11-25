package flagsvc

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/auditkit"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/fsops/memfs"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

// flagGoldenPath holds every flag decision and RPC outcome of the package over an input matrix. It was recorded
// from the pre-move code in server/services (origin/main at 826660d15), so a pass means the move changed no
// decision byte for byte. Regenerate (only for an intended behaviour change) with FLAGSVC_GOLDEN_OUT=<path>.
const flagGoldenPath = "testdata/flags.golden.gz"

func TestFlagBehaviour_ShouldMatchTheRecordedGolden_WhenTheMatrixIsReplayed(t *testing.T) {
	got := dumpFlagBehaviour(t)
	if out := os.Getenv("FLAGSVC_GOLDEN_OUT"); out != "" {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, err := zw.Write([]byte(got))
		require.NoError(t, err)
		require.NoError(t, zw.Close())
		require.NoError(t, os.WriteFile(out, buf.Bytes(), 0o644))
		return
	}
	f, err := os.Open(flagGoldenPath)
	require.NoError(t, err)
	defer f.Close()
	zr, err := gzip.NewReader(f)
	require.NoError(t, err)
	want, err := io.ReadAll(zr)
	require.NoError(t, err)
	if string(want) != got {
		gl, wl := strings.Split(got, "\n"), strings.Split(string(want), "\n")
		for i := 0; i < len(gl) && i < len(wl); i++ {
			if gl[i] != wl[i] {
				t.Fatalf("flag behaviour drift at line %d:\n got: %s\nwant: %s", i+1, gl[i], wl[i])
			}
		}
		t.Fatalf("flag behaviour drift: got %d lines, want %d", len(gl), len(wl))
	}
}

func dumpFlagBehaviour(t *testing.T) string {
	var b strings.Builder
	svctest.WithIsolatedStateDir(t)
	reg := FlagRegistry()
	dumpRegistry(&b, reg)
	dumpMutations(t, &b, reg)
	dumpControllerScenario(t, &b)
	dumpAuditScenario(t, &b)
	dumpReaderMatrix(&b, reg)
	dumpAttentionAndEngine(&b, reg)
	return b.String()
}

func dumpRegistry(b *strings.Builder, reg *featureflags.Registry) {
	for i, s := range reg.Specs() {
		fmt.Fprintf(b, "spec %d %q default=%v scopes=%q desc=%q\n", i, s.Name, s.Default, s.Scopes, s.Description)
	}
}

func resetFlagConfig(t *testing.T) {
	require.NoError(t, config.UpdateConfig(func(c *config.Config) error {
		c.FeatureFlags, c.FeatureFlagScopes = nil, nil
		return nil
	}))
}

func fmtErr(err error) string {
	var ce *connect.Error
	if errors.As(err, &ce) {
		return fmt.Sprintf("err code=%s msg=%q", ce.Code(), ce.Message())
	}
	return "err " + err.Error()
}

func hashMsg(m proto.Message) string {
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(m)
	if err != nil {
		return "marshal-error"
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw))[:16]
}

func flagEntry(svc *FeatureFlagService, name string) string {
	resp, err := svc.GetFeatureFlags(context.Background(), connect.NewRequest(&sessionv1.GetFeatureFlagsRequest{}))
	if err != nil {
		return fmtErr(err)
	}
	body := &sessionv1.GetFeatureFlagsResponse{Flags: resp.Msg.Flags}
	for _, f := range resp.Msg.Flags {
		if f.Name == name {
			return fmt.Sprintf("entry=%s enabled=%v detail=%q scopes=%s all=%s etaglen=%d", hashMsg(f), f.Enabled, f.StatusDetail, scopeList(f.Scopes), hashMsg(body), len(resp.Msg.Etag))
		}
	}
	return "entry-missing all=" + hashMsg(body)
}

// scopeList renders the overrides without proto's String, whose spacing is deliberately unstable.
func scopeList(scopes []*sessionv1.FeatureFlagScopeOverride) string {
	parts := make([]string, len(scopes))
	for i, sc := range scopes {
		parts[i] = fmt.Sprintf("%s=%v", sc.Scope, sc.Enabled)
	}
	return strings.Join(parts, ",")
}

func flagRequests(name string, scopes []string) []*sessionv1.UpdateFeatureFlagRequest {
	m := sessionv1.FlagMutation_FLAG_MUTATION_UNSPECIFIED
	set, dis := sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED, sessionv1.FlagMutation_FLAG_MUTATION_SET_DISABLED
	clear, reset := sessionv1.FlagMutation_FLAG_MUTATION_CLEAR_SCOPE, sessionv1.FlagMutation_FLAG_MUTATION_RESET_GLOBAL
	mk := func(mut sessionv1.FlagMutation, scope string, en bool) *sessionv1.UpdateFeatureFlagRequest {
		return &sessionv1.UpdateFeatureFlagRequest{Name: name, Mutation: mut, Scope: scope, Enabled: en}
	}
	reqs := []*sessionv1.UpdateFeatureFlagRequest{
		mk(m, "", true), mk(m, "", false), mk(set, GlobalScopeLiteral, true), mk(dis, GlobalScopeLiteral, false),
		mk(set, "", true), mk(reset, "", false), mk(reset, "x", false), mk(clear, "", false), mk(clear, GlobalScopeLiteral, false),
		mk(m, "zz:none", true), mk(set, "zz:none", true), mk(clear, "zz:none", false), mk(sessionv1.FlagMutation(99), "", true),
	}
	for _, sc := range scopes {
		reqs = append(reqs, mk(set, sc, true), mk(dis, sc, false), mk(m, sc, true), mk(clear, sc, false), mk(set, sc, true))
	}
	return append(reqs, mk(m, "", true), mk(reset, "", false))
}

func dumpMutations(t *testing.T, b *strings.Builder, reg *featureflags.Registry) {
	svc := NewFeatureFlagService()
	names := append([]string{"", "no-such-flag"}, reg.Names()...)
	for _, name := range names {
		resetFlagConfig(t)
		spec, _ := reg.Lookup(name)
		for i, req := range flagRequests(name, spec.Scopes) {
			resp, err := svc.UpdateFeatureFlag(context.Background(), connect.NewRequest(req))
			var out string
			if err != nil {
				out = fmtErr(err)
			} else {
				out = "ok " + hashMsg(resp.Msg)
			}
			fmt.Fprintf(b, "mut %q #%d mut=%d scope=%q en=%v -> %s | %s\n", name, i, req.Mutation, req.Scope, req.Enabled, out, flagEntry(svc, name))
		}
	}
	resetFlagConfig(t)
}

type recordingController struct {
	mu    sync.Mutex
	on    bool
	calls []string
}

func (c *recordingController) Enable(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.on = true
	c.calls = append(c.calls, "enable")
	return nil
}

func (c *recordingController) Disable() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.on = false
	c.calls = append(c.calls, "disable")
	return nil
}

func (c *recordingController) IsEnabled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.on
}

func (c *recordingController) ApplyScope(scope string, enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, fmt.Sprintf("scope %s=%v", scope, enabled))
	return nil
}

type recordingObserver struct{ names []string }

func (o *recordingObserver) OnFlagChanged(name string) { o.names = append(o.names, name) }

func dumpControllerScenario(t *testing.T, b *strings.Builder) {
	resetFlagConfig(t)
	svc := NewFeatureFlagService()
	WireNotificationActionFlags(svc)
	ctrl := &recordingController{}
	obs := &recordingObserver{}
	svc.SetFeatureController("backlog", ctrl)
	svc.SetFlagObserver(obs)
	svc.SetStatusDetailProvider("browser-passthrough", func() string { return "provider detail" })
	svc.AddStatusDetailSource("browser-passthrough", func() string { return "extra detail" })
	svc.AddConfigStatusDetailSource("browser-passthrough", func(*config.Config) string { return "config detail" })
	svc.SetEnableGuard("backlog:conversation-view", func() string { return "guard refuses" })
	names := []string{"backlog", "browser-passthrough", "backlog:conversation-view", BackgroundActivityFeedFlagName, NotificationInlineActionsFlagName, AskUserQuestionStructuredFlagName, AgentWakeTriggersFlagName}
	for _, name := range names {
		for i, en := range []bool{true, false, true, false} {
			_, err := svc.UpdateFeatureFlag(context.Background(), connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{Name: name, Enabled: en}))
			res := "ok"
			if err != nil {
				res = fmtErr(err)
			}
			fmt.Fprintf(b, "ctrl %q #%d en=%v -> %s | %s\n", name, i, en, res, flagEntry(svc, name))
		}
	}
	fmt.Fprintf(b, "ctrl calls=%q observer=%q\n", ctrl.calls, obs.names)
	resetFlagConfig(t)
}

func dumpAuditScenario(t *testing.T, b *strings.Builder) {
	resetFlagConfig(t)
	mfs := memfs.New()
	sink := auditkit.NewAuditSink(func() (string, error) { return "/cfg", nil },
		auditkit.WithAuditFS(mfs), auditkit.WithAuditClock(func() time.Time { return time.Unix(1700000000, 0).UTC() }))
	svc := NewFeatureFlagService()
	policies := map[string]featureflags.FlagAuditPolicy{
		"backlog:conversation-view":    {},
		ProgramCLIFlagProbeFlagName:    {Loosening: func(enabled bool) bool { return !enabled }},
		"backlog:sdd-default-pipeline": {Loosening: func(enabled bool) bool { return !enabled }, EscapeHatch: true},
		"unfinished:mmap-index":        {Loosening: func(enabled bool) bool { return enabled }, EscapeHatchScopes: []string{"kind:review"}},
		"browser-passthrough":          {Loosening: func(enabled bool) bool { return enabled }},
	}
	svc.SetAudit(sink, policies)
	svc.MergeAuditPolicies(NotificationActionsFlagAuditPolicies())
	svc.MergeAuditPolicies(SessionMultiPRFlagAuditPolicies())
	audited := make([]string, 0, len(policies)+len(NotificationActionsFlagAuditPolicies())+len(SessionMultiPRFlagAuditPolicies()))
	for name := range policies {
		audited = append(audited, name)
	}
	for name := range NotificationActionsFlagAuditPolicies() {
		audited = append(audited, name)
	}
	for name := range SessionMultiPRFlagAuditPolicies() {
		audited = append(audited, name)
	}
	sort.Strings(audited)
	for _, name := range audited {
		for i, en := range []bool{true, false, true} {
			_, err := svc.UpdateFeatureFlag(context.Background(), connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{Name: name, Enabled: en}))
			res := "ok"
			if err != nil {
				res = fmtErr(err)
			}
			fmt.Fprintf(b, "audit %q #%d en=%v -> %s\n", name, i, en, res)
		}
	}
	sink.Close()
	raw, _ := mfs.Get("/cfg/audit/" + auditkit.AuditFileName)
	fmt.Fprintf(b, "audit file:\n%s\n", normalizeAudit(string(raw)))
	for _, name := range []string{config.SessionMultiPRFlag, config.ArtifactScanV2Flag} {
		p := SessionMultiPRFlagAuditPolicies()[name]
		fmt.Fprintf(b, "policy %q loosen(true)=%v loosen(false)=%v hatch(scope)=%v hatch(refresher)=%v\n", name, p.Loosening(true), p.Loosening(false), p.EscapeHatchFor("x"), p.EscapeHatchFor(config.SessionMultiPRScopeRefresherOff))
	}
	resetFlagConfig(t)
}

func ptrBool(v bool) *bool { return &v }

var auditVolatile = regexp.MustCompile(`"(change_id|boot_id|boot_ts)":"[^"]*"`)

// normalizeAudit blanks the per-process random ids and sorts the lines: the queued tightening lines are written by a
// drain goroutine, so their order against the synchronous ones is not deterministic.
func normalizeAudit(raw string) string {
	lines := strings.Split(strings.TrimSpace(auditVolatile.ReplaceAllString(raw, `"$1":"-"`)), "\n")
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func tristates() []*bool { return []*bool{nil, ptrBool(true), ptrBool(false)} }

func dumpReaderMatrix(b *strings.Builder, reg *featureflags.Registry) {
	var dps []string
	dpSeen := map[string]bool{"": true}
	dps = append(dps, "")
	for _, s := range reg.Specs() {
		for _, sc := range s.Scopes {
			if !dpSeen[sc] {
				dpSeen[sc] = true
				dps = append(dps, sc)
			}
		}
	}
	dps = append(dps, "stage:render", "stage:answer", "stage:hook_reply", "dispatcher", "channel:remote", "channel:remote:tierB")
	evs := []FlagEvidence{{}, {S1HookReplyPass: true}, {S6TierBPass: true}, {S1HookReplyPass: true, S6TierBPass: true}}
	r := FlagReader{reg}
	for _, s := range reg.Specs() {
		for _, g := range tristates() {
			cfg := &config.Config{}
			if g != nil {
				cfg.FeatureFlags = map[string]bool{s.Name: *g}
			}
			fmt.Fprintf(b, "enabled %q g=%s -> %v detail=%q\n", s.Name, triName(g), r.Enabled(cfg, s.Name), r.GlobalOffStatusDetail(cfg))
		}
	}
	for _, name := range []string{BackgroundActivityFeedFlagName, NotificationInlineActionsFlagName, AskUserQuestionStructuredFlagName, AgentWakeTriggersFlagName} {
		for _, dp := range dps {
			for _, g := range tristates() {
				for _, sv := range tristates() {
					cfg := &config.Config{}
					if g != nil {
						cfg.FeatureFlags = map[string]bool{name: *g}
					}
					if sv != nil {
						cfg.FeatureFlagScopes = map[string]map[string]bool{name: {dp: *sv}}
					}
					for ei, ev := range evs {
						fmt.Fprintf(b, "resolve %q dp=%q g=%s s=%s ev=%d -> %v\n", name, dp, triName(g), triName(sv), ei, r.ResolveEffectiveFlag(cfg, ev, name, dp))
					}
				}
			}
		}
	}
	for route := AnswerRoute(0); route <= RouteHookReply+1; route++ {
		for _, g := range tristates() {
			for _, sv := range tristates() {
				cfg := &config.Config{FeatureFlags: map[string]bool{}, FeatureFlagScopes: map[string]map[string]bool{}}
				if g != nil {
					cfg.FeatureFlags[NotificationInlineActionsFlagName] = *g
					cfg.FeatureFlags[AskUserQuestionStructuredFlagName] = *g
				}
				if sv != nil {
					cfg.FeatureFlagScopes[NotificationInlineActionsFlagName] = map[string]bool{ScopePrefixAction + ActionKindAnswerQuestion: *sv, ScopeChannelRemote: *sv}
					cfg.FeatureFlagScopes[AskUserQuestionStructuredFlagName] = map[string]bool{ScopeStageRender: *sv, ScopeStageAnswer: *sv, ScopeStageHookReply: *sv}
				}
				for ei, ev := range evs {
					fmt.Fprintf(b, "answer route=%d g=%s s=%s ev=%d -> %v\n", route, triName(g), triName(sv), ei, r.ResolveAnswerQuestion(cfg, ev, route))
				}
			}
		}
	}
}

func triName(v *bool) string {
	if v == nil {
		return "unset"
	}
	return fmt.Sprint(*v)
}

func dumpAttentionAndEngine(b *strings.Builder, reg *featureflags.Registry) {
	names := []string{AttentionProseQuestionDetectionFlagName, SessionEventLogFlagName, AttentionAckBypassFixFlagName, AttentionIdleClockOutputFlagName, AttentionProseQuestionLLMTiebreakFlagName, AttentionInboxConsolidationFlagName}
	for mask := 0; mask < 1<<len(names); mask++ {
		cfg := &config.Config{FeatureFlags: map[string]bool{}}
		for i, n := range names {
			if mask&(1<<i) != 0 {
				cfg.FeatureFlags[n] = true
			}
		}
		fmt.Fprintf(b, "attention mask=%d -> %+v\n", mask, ResolveAttentionFlags(cfg, reg))
	}
	for _, sh := range tristates() {
		for _, en := range tristates() {
			cfg := &config.Config{FeatureFlags: map[string]bool{}}
			if sh != nil {
				cfg.FeatureFlags[config.WorkflowEngineShadowFeatureFlag] = *sh
			}
			if en != nil {
				cfg.FeatureFlags[config.WorkflowEngineEnforceFeatureFlag] = *en
			}
			fmt.Fprintf(b, "engine shadow=%s enforce=%s -> %v\n", triName(sh), triName(en), WorkflowEngineModeFromConfig(cfg, reg))
		}
	}
	for _, g := range tristates() {
		cfg := &config.Config{}
		if g != nil {
			cfg.FeatureFlagScopes = map[string]map[string]bool{config.HiddenSessionGateFeatureFlag: {"kind:review": *g, "kind:triage": !*g}}
		}
		fmt.Fprintf(b, "kinds g=%s -> %v detail=%q\n", triName(g), ExplicitOffKinds(cfg), KindOffStatusDetail(cfg))
	}
	fmt.Fprintf(b, "program probe -> %v\n", ProgramCLIFlagProbeEnabled(reg))
}
