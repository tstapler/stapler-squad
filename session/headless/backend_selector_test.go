package headless

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type fakeBackend struct {
	name   string
	caps   Caps
	up     bool
	cost   float64
	priced bool
	calls  int
	gotOpt CallOptions
	err    error
}

func (f *fakeBackend) Name() string       { return f.name }
func (f *fakeBackend) Capabilities() Caps { return f.caps }
func (f *fakeBackend) Available() bool    { return f.up }
func (f *fakeBackend) CallBlocking(_ context.Context, _ FeatureKey, _, _ string, opts CallOptions, sink CostSink) (string, error) {
	f.calls++
	f.gotOpt = opts
	if f.err != nil {
		return "", f.err
	}
	if f.priced {
		sink(f.cost, true)
	}
	return f.name, nil
}

func newTestSelector(set *BackendSettings, bs ...Backend) (*Selector, *[]Resolution) {
	s := NewSelector(func() BackendSettings { return *set })
	for _, b := range bs {
		s.Register(b)
	}
	var recs []Resolution
	s.SetFallbackRecorder(func(r Resolution) { recs = append(recs, r) })
	return s, &recs
}

func TestSelectorPrecedenceAndFallbackMatrix(t *testing.T) {
	claude := &fakeBackend{name: BackendClaude, caps: ClaudeCaps, up: true}
	gem := &fakeBackend{name: BackendGemini, caps: Caps{WorkDir: true}, up: true}
	down := &fakeBackend{name: BackendConsolette, caps: ClaudeCaps, up: false}

	cases := []struct {
		name       string
		set        BackendSettings
		feature    FeatureKey
		stage      string
		need       Caps
		wantUsed   string
		wantReason string
	}{
		{"default claude", BackendSettings{}, FeatureKeySummarize, "", Caps{}, BackendClaude, ""},
		{"global default", BackendSettings{Default: BackendGemini}, FeatureKeySummarize, "", Caps{}, BackendGemini, ""},
		{"per-feature beats default", BackendSettings{Default: BackendGemini, PerFeature: map[string]string{"summarize": BackendClaude}}, FeatureKeySummarize, "", Caps{}, BackendClaude, ""},
		{"stage beats per-feature", BackendSettings{PerFeature: map[string]string{"summarize": BackendClaude}}, FeatureKeySummarize, BackendGemini, Caps{}, BackendGemini, ""},
		{"dynamic feature key prefix", BackendSettings{PerFeature: map[string]string{"gate-custom-check": BackendGemini}}, "gate-custom-check:abc", "", Caps{}, BackendGemini, ""},
		{"unavailable falls back", BackendSettings{Default: BackendConsolette}, FeatureKeySummarize, "", Caps{}, BackendClaude, "consolette_unavailable"},
		{"unknown falls back", BackendSettings{Default: "nope"}, FeatureKeySummarize, "", Caps{}, BackendClaude, "unsupported_program"},
		{"missing resume falls back", BackendSettings{Default: BackendGemini}, FeatureKeySummarize, "", Caps{Resume: true}, BackendClaude, "gemini_lacks_resume"},
		{"missing tool restriction falls back", BackendSettings{Default: BackendGemini}, FeatureKeyTriage, "", Caps{ToolRestriction: true, WorkDir: true}, BackendClaude, "gemini_lacks_tool_restriction"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			set := tc.set
			s, recs := newTestSelector(&set, claude, gem, down)
			b, res := s.Resolve(tc.feature, tc.stage, tc.need)
			if b == nil || b.Name() != tc.wantUsed || res.Used != tc.wantUsed {
				t.Fatalf("used = %v/%q, want %q", b, res.Used, tc.wantUsed)
			}
			if res.FallbackReason != tc.wantReason {
				t.Errorf("reason = %q, want %q", res.FallbackReason, tc.wantReason)
			}
			if (tc.wantReason != "") != (len(*recs) == 1) {
				t.Errorf("recorded %d fallbacks for reason %q", len(*recs), tc.wantReason)
			}
		})
	}
}

func TestSelectorSettingsApplyLive(t *testing.T) {
	set := BackendSettings{}
	claude := &fakeBackend{name: BackendClaude, caps: ClaudeCaps, up: true}
	gem := &fakeBackend{name: BackendGemini, caps: Caps{}, up: true}
	s, _ := newTestSelector(&set, claude, gem)
	if b, _ := s.Resolve(FeatureKeySummarize, "", Caps{}); b.Name() != BackendClaude {
		t.Fatal("expected claude before edit")
	}
	set.PerFeature = map[string]string{"summarize": BackendGemini}
	if b, _ := s.Resolve(FeatureKeySummarize, "", Caps{}); b.Name() != BackendGemini {
		t.Fatal("settings edit must apply without restart")
	}
}

func TestSelectorFailsClosedWithoutClaude(t *testing.T) {
	set := BackendSettings{Default: BackendGemini}
	gem := &fakeBackend{name: BackendGemini, caps: Caps{}, up: false}
	s, recs := newTestSelector(&set, gem)
	c := &SelectingClient{Selector: s}
	_, err := c.CallBlocking(context.Background(), FeatureKeySummarize, "", "x", CallOptions{}, nil)
	if !errors.Is(err, ErrNoBackend) {
		t.Fatalf("err = %v, want ErrNoBackend", err)
	}
	if len(*recs) != 1 {
		t.Fatalf("fallback not recorded")
	}
}

func TestSelectingClientTranslatesModelAndReportsCost(t *testing.T) {
	set := BackendSettings{Default: BackendGemini}
	claude := &fakeBackend{name: BackendClaude, caps: ClaudeCaps, up: true, priced: true, cost: 0.5}
	gem := &fakeBackend{name: BackendGemini, caps: Caps{SystemPrompt: true}, up: true}
	s, _ := newTestSelector(&set, claude, gem)
	c := &SelectingClient{Selector: s}

	var usd float64
	var priced, fired int
	sink := func(u float64, p bool) {
		fired++
		usd = u
		if p {
			priced++
		}
	}
	if _, err := c.CallBlocking(context.Background(), FeatureKeySummarize, "sys", "u", CallOptions{Model: "claude-sonnet-4-5"}, sink); err != nil {
		t.Fatal(err)
	}
	if gem.gotOpt.Model != "gemini-2.5-pro" {
		t.Errorf("gemini model = %q", gem.gotOpt.Model)
	}
	if fired != 1 || priced != 0 || usd != 0 {
		t.Errorf("unpriced backend must report unpriced once: fired=%d priced=%d usd=%v", fired, priced, usd)
	}

	set.Default = BackendClaude
	fired, priced = 0, 0
	if _, err := c.CallBlocking(context.Background(), FeatureKeySummarize, "", "u", CallOptions{Model: "claude-sonnet-4-5"}, sink); err != nil {
		t.Fatal(err)
	}
	if claude.gotOpt.Model != "claude-sonnet-4-5" || fired != 1 || priced != 1 || usd != 0.5 {
		t.Errorf("claude passthrough/cost wrong: model=%q fired=%d priced=%d usd=%v", claude.gotOpt.Model, fired, priced, usd)
	}
}

func TestTranslateModel(t *testing.T) {
	set := BackendSettings{ModelMaps: map[string]map[string]string{
		BackendConsolette: {"sonnet": "router-large"},
		BackendGemini:     {"haiku": ""},
	}}
	cases := []struct{ backend, in, want string }{
		{BackendClaude, "claude-opus-4-1", "claude-opus-4-1"},
		{BackendConsolette, "claude-sonnet-4-5", "router-large"},
		{BackendConsolette, "claude-haiku-4-5", "claude-haiku-4-5"},
		{BackendGemini, "haiku", ""},
		{BackendGemini, "claude-opus-4-1", "gemini-2.5-pro"},
		{BackendAgy, "claude-sonnet-4-5", ""},
		{BackendAgy, "", ""},
	}
	for _, tc := range cases {
		if got := set.TranslateModel(tc.backend, tc.in); got != tc.want {
			t.Errorf("TranslateModel(%s,%q) = %q, want %q", tc.backend, tc.in, got, tc.want)
		}
	}
}

func TestCapsNeeded(t *testing.T) {
	got := CapsNeeded("sys", CallOptions{WorkDir: "/x", AllowedTools: "Read"})
	if !got.SystemPrompt || !got.WorkDir || !got.ToolRestriction || got.Resume {
		t.Errorf("CapsNeeded = %+v", got)
	}
	if CapsNeeded(" ", CallOptions{}) != (Caps{}) {
		t.Error("blank system prompt and empty opts need nothing")
	}
}

func TestBackendCapabilityTable(t *testing.T) {
	cli := NewCLIBackend(KnownCLIBackendSpecs[0])
	cons := newConsoletteBackend(func() BackendSettings { return BackendSettings{} }, nil, nil)
	cases := []struct {
		b    Backend
		name string
		want Caps
	}{
		{NewClaudeBackend(&Pool{}), BackendClaude, ClaudeCaps},
		{cons, BackendConsolette, ClaudeCaps},
		{cli, BackendAgy, Caps{WorkDir: true}},
		{NewCLIBackend(KnownCLIBackendSpecs[1]), BackendOpenCode, Caps{WorkDir: true}},
		{NewGeminiBackend(&GeminiCaller{}), BackendGemini, Caps{WorkDir: true}},
	}
	for _, tc := range cases {
		if tc.b.Name() != tc.name || tc.b.Capabilities() != tc.want {
			t.Errorf("%s: name=%s caps=%+v", tc.name, tc.b.Name(), tc.b.Capabilities())
		}
	}
	if NewClaudeBackend(nil) != nil || NewGeminiBackend(nil) != nil {
		t.Error("nil inputs must yield untyped nil Backend")
	}
}

func TestConsoletteAvailabilityFollowsRouterAndURL(t *testing.T) {
	set := BackendSettings{}
	up := false
	var probed []string
	now := time.Unix(0, 0)
	c := newConsoletteBackend(func() BackendSettings { return set }, nil, func(_ context.Context, u string) bool {
		probed = append(probed, u)
		return up
	})
	c.now = func() time.Time { return now }

	if c.Available() {
		t.Fatal("router down must report unavailable")
	}
	up = true
	if c.Available() {
		t.Fatal("result is TTL-cached")
	}
	now = now.Add(consoletteProbeTTL + time.Second)
	if !c.Available() {
		t.Fatal("expected available after TTL")
	}
	set.ConsoletteBaseURL = "http://example.test:1/"
	if !c.Available() || probed[len(probed)-1] != "http://example.test:1" {
		t.Fatalf("base URL change must re-probe the new URL, probed=%v", probed)
	}
	if probed[0] != DefaultConsoletteBaseURL {
		t.Errorf("default URL = %q", probed[0])
	}
}

func TestConsoletteCallUsesPoolForCurrentURLAndReportsUnpriced(t *testing.T) {
	set := BackendSettings{ConsoletteBaseURL: "http://a"}
	pools := map[string]*fakeBackend{}
	c := newConsoletteBackend(func() BackendSettings { return set }, func(u string) (PoolClient, error) {
		f := &fakeBackend{name: u, up: true, priced: true, cost: 9}
		pools[u] = f
		return f, nil
	}, nil)
	var priced bool
	sink := func(_ float64, p bool) { priced = p }
	if _, err := c.CallBlocking(context.Background(), FeatureKeySummarize, "", "x", CallOptions{}, sink); err != nil {
		t.Fatal(err)
	}
	set.ConsoletteBaseURL = "http://b"
	if _, err := c.CallBlocking(context.Background(), FeatureKeySummarize, "", "x", CallOptions{}, sink); err != nil {
		t.Fatal(err)
	}
	if pools["http://a"].calls != 1 || pools["http://b"].calls != 1 || priced {
		t.Errorf("per-URL pools / unpriced reporting wrong: %+v priced=%v", pools, priced)
	}
}

func TestCLIBackendInvocation(t *testing.T) {
	var gotBin string
	var gotArgs []string
	var gotStdin bool
	b := &CLIBackend{
		spec:     KnownCLIBackendSpecs[0],
		lookPath: func(s string) (string, error) { return "/bin/" + s, nil },
		run: func(_ context.Context, bin string, args []string, stdin io.Reader, _ string) ([]byte, error) {
			gotBin, gotArgs, gotStdin = bin, args, stdin != nil
			return []byte(" ok \n"), nil
		},
	}
	var priced = true
	out, err := b.CallBlocking(context.Background(), FeatureKeySummarize, "sys", "user", CallOptions{}, func(_ float64, p bool) { priced = p })
	if err != nil || out != "ok" || priced {
		t.Fatalf("out=%q err=%v priced=%v", out, err, priced)
	}
	if gotBin != "/bin/agy" || gotStdin || len(gotArgs) != 2 || gotArgs[0] != "--print" || gotArgs[1] != "sys\n\n---\n\nuser" {
		t.Errorf("bin=%q args=%q stdin=%v", gotBin, gotArgs, gotStdin)
	}
}

func TestCLIBackendUnavailableWhenBinaryMissing(t *testing.T) {
	b := NewCLIBackend(CLIBackendSpec{Name: "x", Binary: "definitely-not-a-real-binary-xyz"})
	if b.Available() {
		t.Fatal("missing binary must be unavailable")
	}
	if _, err := b.CallBlocking(context.Background(), FeatureKeySummarize, "", "p", CallOptions{}, nil); err == nil || !strings.Contains(err.Error(), "backend") {
		t.Fatalf("err = %v", err)
	}
}
