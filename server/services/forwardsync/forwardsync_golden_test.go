package forwardsync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// The golden was recorded from the pre-move subscribers in server/services (origin/main)
// with FORWARDSYNC_GOLDEN_RECORD=1; it names only identifiers present in both packages,
// so the same file replays against either. Every line is something the sync writes to
// GitHub (plugin calls, comment bodies, plugin config) or to the backlog store (status,
// watermark, dedup status, recorded source failures): any diff is a behaviour change.
const goldenPath = "testdata/forwardsync.golden"

type goldenLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *goldenLog) add(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, "    "+fmt.Sprintf(format, args...))
}

func (l *goldenLog) drain() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.lines
	l.lines = nil
	return out
}

func gts(t time.Time) string {
	if t.IsZero() {
		return "zero"
	}
	// persistForwardSyncWatermark falls back to local time when the plugin gave none.
	if d := time.Since(t); d > -time.Minute && d < time.Minute {
		return "local-now"
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// gCloser records every call; it has ReadIssueState so the already-closed pre-check runs.
type gCloser struct {
	id         string
	log        *goldenLog
	closeErr   error
	commentErr error
	stateErr   error
	state      string
	stateAt    time.Time
	closeAt    time.Time
}

func (p *gCloser) PluginID() string { return p.id }
func (p *gCloser) Fetch(_ context.Context, _ session.PluginConfig, cursor string) ([]session.ExternalItem, string, error) {
	return nil, cursor, nil
}
func (p *gCloser) MapToBacklogItem(_ session.ExternalItem, _ string) backlogdto.BacklogItemData {
	return backlogdto.BacklogItemData{}
}
func (p *gCloser) CloseIssue(_ context.Context, cfg session.PluginConfig, externalID string, labels []string, closeLabel string) (time.Time, error) {
	p.log.add("close cfg=%s ext=%q labels=%q closeLabel=%q", cfg.Raw, externalID, labels, closeLabel)
	if p.closeErr != nil {
		return time.Time{}, p.closeErr
	}
	return p.closeAt, nil
}
func (p *gCloser) PostIssueComment(_ context.Context, cfg session.PluginConfig, externalID, body string) error {
	p.log.add("comment cfg=%s ext=%q body=%q", cfg.Raw, externalID, body)
	return p.commentErr
}
func (p *gCloser) ReadIssueState(_ context.Context, cfg session.PluginConfig, externalID string) (string, time.Time, error) {
	p.log.add("readstate cfg=%s ext=%q", cfg.Raw, externalID)
	return p.state, p.stateAt, p.stateErr
}

// gCloserNoReader hides ReadIssueState behind a delegating wrapper.
type gCloserNoReader struct{ c *gCloser }

func (p gCloserNoReader) PluginID() string { return p.c.id }
func (p gCloserNoReader) Fetch(ctx context.Context, cfg session.PluginConfig, cursor string) ([]session.ExternalItem, string, error) {
	return p.c.Fetch(ctx, cfg, cursor)
}
func (p gCloserNoReader) MapToBacklogItem(i session.ExternalItem, s string) backlogdto.BacklogItemData {
	return p.c.MapToBacklogItem(i, s)
}
func (p gCloserNoReader) CloseIssue(ctx context.Context, cfg session.PluginConfig, e string, l []string, c string) (time.Time, error) {
	return p.c.CloseIssue(ctx, cfg, e, l, c)
}
func (p gCloserNoReader) PostIssueComment(ctx context.Context, cfg session.PluginConfig, e, b string) error {
	return p.c.PostIssueComment(ctx, cfg, e, b)
}

// gUpdater is the Linear/JIRA shape: a named target state instead of a binary close.
type gUpdater struct {
	id         string
	log        *goldenLog
	updateErr  error
	commentErr error
	updatedAt  time.Time
}

func (p *gUpdater) PluginID() string { return p.id }
func (p *gUpdater) Fetch(_ context.Context, _ session.PluginConfig, cursor string) ([]session.ExternalItem, string, error) {
	return nil, cursor, nil
}
func (p *gUpdater) MapToBacklogItem(_ session.ExternalItem, _ string) backlogdto.BacklogItemData {
	return backlogdto.BacklogItemData{}
}
func (p *gUpdater) UpdateIssueState(_ context.Context, cfg session.PluginConfig, externalID, target string) (time.Time, error) {
	p.log.add("updatestate cfg=%s ext=%q target=%q", cfg.Raw, externalID, target)
	if p.updateErr != nil {
		return time.Time{}, p.updateErr
	}
	return p.updatedAt, nil
}
func (p *gUpdater) PostIssueComment(_ context.Context, cfg session.PluginConfig, externalID, body string) error {
	p.log.add("comment cfg=%s ext=%q body=%q", cfg.Raw, externalID, body)
	return p.commentErr
}

// gPlain implements session.ItemSourcePlugin only (the PR plugin shape).
type gPlain struct{ id string }

func (p gPlain) PluginID() string { return p.id }
func (p gPlain) Fetch(_ context.Context, _ session.PluginConfig, cursor string) ([]session.ExternalItem, string, error) {
	return nil, cursor, nil
}
func (p gPlain) MapToBacklogItem(_ session.ExternalItem, _ string) backlogdto.BacklogItemData {
	return backlogdto.BacklogItemData{}
}

type gProgress struct {
	gPlain
	log       *goldenLog
	err       error
	updatedAt time.Time
}

func (p *gProgress) PostProgressComment(_ context.Context, cfg session.PluginConfig, externalID, body string) (time.Time, error) {
	p.log.add("progress cfg=%s ext=%q body=%q", cfg.Raw, externalID, body)
	return p.updatedAt, p.err
}

func scrub(s, itemID string) string {
	if itemID == "" {
		return s
	}
	return strings.ReplaceAll(s, itemID, "<ITEM>")
}

// observe prints the store effects: status, watermark, dedup status and recorded failures.
func observe(t *testing.T, g *strings.Builder, storage *session.Storage, itemID, sourceID string) {
	t.Helper()
	got, err := storage.GetBacklogItem(context.Background(), itemID)
	require.NoError(t, err)
	wm := "nil"
	if got.GitHubSyncedIssueUpdatedAt != nil {
		wm = gts(*got.GitHubSyncedIssueUpdatedAt)
	}
	fmt.Fprintf(g, "    store status=%s watermark=%s lastProgress=%q\n", got.Status, wm, got.GitHubLastProgressCommentStatus)
	if sourceID != "" {
		hist, truncated, err := storage.ListSourceSyncEvents(context.Background(), sourceID)
		require.NoError(t, err)
		fmt.Fprintf(g, "    failures n=%d truncated=%v\n", len(hist), truncated)
		for _, h := range hist {
			fmt.Fprintf(g, "      failure %q errored=%d\n", scrub(h.ErrorMessage, itemID), h.ItemsErrored)
		}
	}
}

func flush(g *strings.Builder, l *goldenLog) {
	for _, s := range l.drain() {
		g.WriteString(s + "\n")
	}
}

type fwdCase struct {
	name       string
	plugin     string // closer, closer-noreader, updater, plain, none
	enabled    bool
	closeLabel string
	externalID string
	srcConfig  string
	labels     []string
	host       string
	pr         string
	closeErr   error
	commentErr error
	stateErr   error
	updateErr  error
	state      string
	stateAt    time.Time
	closeAt    time.Time
}

var (
	gT1 = time.Date(2026, 3, 4, 5, 6, 7, 123456789, time.UTC)
	gT2 = time.Date(2026, 7, 8, 9, 10, 11, 0, time.UTC)
)

func forwardCases() []fwdCase {
	out := make([]fwdCase, 0, 38)
	for _, plugin := range []string{"closer", "closer-noreader", "updater", "plain", "none"} {
		for _, enabled := range []bool{true, false} {
			for _, ext := range []string{"42", ""} {
				out = append(out, fwdCase{name: fmt.Sprintf("%s enabled=%v ext=%q", plugin, enabled, ext), plugin: plugin, enabled: enabled, externalID: ext, closeLabel: "shipped", labels: []string{"bug", "p1"}, closeAt: gT1, host: "host-a", pr: "https://github.com/o/r/pull/9"})
			}
		}
	}
	out = append(out,
		fwdCase{name: "closer no label no host no pr zero ts", plugin: "closer", enabled: true, externalID: "7"},
		fwdCase{name: "closer host only", plugin: "closer", enabled: true, externalID: "7", host: "h1", closeAt: gT2},
		fwdCase{name: "closer pr only", plugin: "closer", enabled: true, externalID: "7", pr: "https://github.com/o/r/pull/3", closeAt: gT2},
		fwdCase{name: "closer host and pr", plugin: "closer", enabled: true, externalID: "7", host: "h1", pr: "https://github.com/o/r/pull/3", closeAt: gT2},
		fwdCase{name: "closer close error", plugin: "closer", enabled: true, externalID: "7", closeErr: fmt.Errorf("github: rate limited"), closeLabel: "x"},
		fwdCase{name: "closer comment error keeps watermark", plugin: "closer", enabled: true, externalID: "7", commentErr: fmt.Errorf("comment boom"), closeAt: gT2},
		fwdCase{name: "closer already closed", plugin: "closer", enabled: true, externalID: "7", state: "closed", stateAt: gT1},
		fwdCase{name: "closer already closed zero ts", plugin: "closer", enabled: true, externalID: "7", state: "closed"},
		fwdCase{name: "closer state open", plugin: "closer", enabled: true, externalID: "7", state: "open", stateAt: gT1, closeAt: gT2},
		fwdCase{name: "closer state read error still closes", plugin: "closer", enabled: true, externalID: "7", stateErr: fmt.Errorf("read boom"), closeAt: gT2},
		fwdCase{name: "closer-noreader already closed is ignored", plugin: "closer-noreader", enabled: true, externalID: "7", state: "closed", closeAt: gT2},
		fwdCase{name: "closer bad source config", plugin: "closer", enabled: true, externalID: "7", srcConfig: "not-json", closeAt: gT2},
		fwdCase{name: "closer rich source config", plugin: "closer", enabled: true, externalID: "7", srcConfig: `{"owner":"o","repo":"r","host":"github.com"}`, closeAt: gT2},
		fwdCase{name: "updater target state", plugin: "updater", enabled: true, externalID: "ENG-1", closeLabel: "In Review", closeAt: gT1},
		fwdCase{name: "updater empty target", plugin: "updater", enabled: true, externalID: "ENG-1"},
		fwdCase{name: "updater update error", plugin: "updater", enabled: true, externalID: "ENG-1", closeLabel: "Shipped", updateErr: fmt.Errorf(`no workflow state named "Shipped" found`)},
		fwdCase{name: "updater comment error", plugin: "updater", enabled: true, externalID: "ENG-1", closeLabel: "Done", commentErr: fmt.Errorf("c"), closeAt: gT2},
		fwdCase{name: "updater zero ts", plugin: "updater", enabled: true, externalID: "ENG-1", closeLabel: "Done"},
	)
	return out
}

func registerFor(c fwdCase, log *goldenLog) (*session.PluginRegistry, string) {
	registry := session.NewPluginRegistry()
	id := "g_plugin"
	closer := &gCloser{id: id, log: log, closeErr: c.closeErr, commentErr: c.commentErr, stateErr: c.stateErr, state: c.state, stateAt: c.stateAt, closeAt: c.closeAt}
	switch c.plugin {
	case "closer":
		registry.Register(closer)
	case "closer-noreader":
		registry.Register(gCloserNoReader{c: closer})
	case "updater":
		registry.Register(&gUpdater{id: id, log: log, updateErr: c.updateErr, commentErr: c.commentErr, updatedAt: c.closeAt})
	case "plain":
		registry.Register(gPlain{id: id})
	}
	return registry, id
}

func goldenForward(t *testing.T, g *strings.Builder) {
	g.WriteString("== forward sync: source-linked items\n")
	for _, c := range forwardCases() {
		log := &goldenLog{}
		storage := svctest.NewForwardSyncTestStorage(t)
		registry, pluginID := registerFor(c, log)
		cfg := c.srcConfig
		if cfg == "" {
			cfg = "{}"
		}
		src, err := storage.CreateItemSource(context.Background(), backlogdto.ItemSourceData{
			PluginID: pluginID, DisplayName: "s", Config: cfg, Enabled: true,
			ForwardSyncEnabled: c.enabled, ForwardSyncCloseLabel: c.closeLabel,
		})
		require.NoError(t, err)
		item, err := storage.CreateBacklogItem(context.Background(), backlogdto.BacklogItemData{
			Title: "t", Status: string(domain.BacklogStatusDone), SourceID: src.ID, ExternalID: c.externalID,
			Labels: c.labels, ClaimingHostID: c.host,
		})
		require.NoError(t, err)
		if c.pr != "" {
			pr := c.pr
			_, err = storage.UpdateBacklogItem(context.Background(), item.ID, backlogdto.BacklogItemUpdate{PrURL: &pr}, nil)
			require.NoError(t, err)
		}
		handleForwardSyncClose(context.Background(), registry, session.NewSyncLoop(storage, registry), storage, item)
		fmt.Fprintf(g, "case %s\n", c.name)
		flush(g, log)
		observe(t, g, storage, item.ID, src.ID)
	}
}

type srcless struct {
	name     string
	url      string
	token    string
	host     string
	pr       string
	closeErr error
	state    string
	stateAt  time.Time
	closeAt  time.Time
	noReader bool
	nilPlug  bool
}

func goldenSourceless(t *testing.T, g *strings.Builder) {
	g.WriteString("== forward sync: sourceless (imported) items\n")
	cases := []srcless{
		{name: "issue ok", url: "https://github.com/acme/widget/issues/42", token: "tok", closeAt: gT1},
		{name: "issue with pr and host", url: "https://github.com/acme/widget/issues/42", token: "tok", pr: "https://github.com/acme/widget/pull/99", host: "h", closeAt: gT2},
		{name: "issue token miss", url: "https://github.com/acme/widget/issues/42"},
		{name: "issue close error", url: "https://github.com/acme/widget/issues/42", token: "tok", closeErr: fmt.Errorf("401 bad credentials")},
		{name: "issue already closed", url: "https://github.com/acme/widget/issues/42", token: "tok", state: "closed", stateAt: gT2},
		{name: "issue no reader", url: "https://github.com/acme/widget/issues/42", token: "tok", noReader: true, state: "closed", closeAt: gT1},
		{name: "issue no plugin", url: "https://github.com/acme/widget/issues/42", token: "tok", nilPlug: true},
		{name: "pull url", url: "https://github.com/acme/widget/pull/7", token: "tok"},
		{name: "foreign host", url: "https://example.com/acme/widget/issues/7", token: "tok"},
		{name: "ghe host skipped", url: "https://ghe.example.com/acme/widget/issues/7", token: "tok"},
		{name: "empty url", url: "", token: "tok"},
		{name: "garbage url", url: "::::", token: "tok"},
		{name: "issue zero close ts", url: "https://github.com/acme/widget/issues/1", token: "tok"},
	}
	for _, c := range cases {
		log := &goldenLog{}
		storage := svctest.NewForwardSyncTestStorage(t)
		item, err := storage.CreateBacklogItem(context.Background(), backlogdto.BacklogItemData{
			Title: "imported", Status: string(domain.BacklogStatusDone), ExternalURL: c.url, ClaimingHostID: c.host,
		})
		require.NoError(t, err)
		if c.pr != "" {
			pr := c.pr
			_, err = storage.UpdateBacklogItem(context.Background(), item.ID, backlogdto.BacklogItemUpdate{PrURL: &pr}, nil)
			require.NoError(t, err)
		}
		closer := &gCloser{id: "x", log: log, closeErr: c.closeErr, state: c.state, stateAt: c.stateAt, closeAt: c.closeAt}
		deps := sourcelessCloseDeps{tokenFor: func(host string) string { log.add("token host=%q", host); return c.token }}
		switch {
		case c.nilPlug:
		case c.noReader:
			deps.plugin = gCloserNoReader{c: closer}
		default:
			deps.plugin = closer
		}
		registry := session.NewPluginRegistry()
		handleForwardSyncCloseWith(context.Background(), registry, session.NewSyncLoop(storage, registry), storage, item, deps)
		fmt.Fprintf(g, "case %s\n", c.name)
		flush(g, log)
		observe(t, g, storage, item.ID, "")
	}
}

type progCase struct {
	name        string
	newStatus   string
	extURL      string
	hasSource   bool
	extID       string
	host        string
	pr          string
	lastStatus  string
	err         error
	updatedAt   time.Time
	noPlugin    bool
	plainPlugin bool
}

func progCases() []progCase {
	var out []progCase
	for _, st := range []string{"in_progress", "review", "done", "triage"} {
		for _, last := range []string{"", "in_progress", "review"} {
			for _, hasSource := range []bool{true, false} {
				out = append(out, progCase{name: fmt.Sprintf("%s last=%q source=%v", st, last, hasSource), newStatus: st, extURL: "https://github.com/o/r/issues/42", hasSource: hasSource, extID: "42", host: "host-abc", pr: "https://github.com/o/r/pull/5", lastStatus: last, updatedAt: gT1})
			}
		}
	}
	return append(out,
		progCase{name: "in_progress no host", newStatus: "in_progress", extURL: "https://github.com/o/r/issues/42", hasSource: true, extID: "42", updatedAt: gT1},
		progCase{name: "in_progress zero updatedAt", newStatus: "in_progress", extURL: "https://github.com/o/r/issues/42", hasSource: true, extID: "42", host: "h"},
		progCase{name: "in_progress post error", newStatus: "in_progress", extURL: "https://github.com/o/r/issues/42", hasSource: true, extID: "42", err: fmt.Errorf("post boom"), updatedAt: gT1},
		progCase{name: "review no pr skips", newStatus: "review", extURL: "https://github.com/o/r/issues/42", hasSource: true, extID: "42", updatedAt: gT1},
		progCase{name: "review malformed pr", newStatus: "review", extURL: "https://github.com/o/r/issues/42", hasSource: true, extID: "42", pr: "not a url", updatedAt: gT1},
		progCase{name: "review foreign pr", newStatus: "review", extURL: "https://github.com/o/r/issues/42", hasSource: true, extID: "42", pr: "https://example.com/o/r/pull/5", updatedAt: gT1},
		progCase{name: "review issue url as pr", newStatus: "review", extURL: "https://github.com/o/r/issues/42", hasSource: true, extID: "42", pr: "https://github.com/o/r/issues/5", updatedAt: gT1},
		progCase{name: "review zero updatedAt", newStatus: "review", extURL: "https://github.com/o/r/issues/42", hasSource: true, extID: "42", pr: "https://github.com/o/r/pull/5"},
		progCase{name: "no external url", newStatus: "in_progress", hasSource: true, extID: "42", updatedAt: gT1},
		progCase{name: "source without external id uses url", newStatus: "in_progress", extURL: "https://github.com/o/r/issues/77", hasSource: true, updatedAt: gT1},
		progCase{name: "sourceless pull url", newStatus: "in_progress", extURL: "https://github.com/o/r/pull/77", updatedAt: gT1},
		progCase{name: "sourceless ghe url", newStatus: "in_progress", extURL: "https://ghe.example.com/o/r/issues/77", updatedAt: gT1},
		progCase{name: "sourceless no plugin", newStatus: "in_progress", extURL: "https://github.com/o/r/issues/77", noPlugin: true},
		progCase{name: "source plugin not a poster", newStatus: "in_progress", extURL: "https://github.com/o/r/issues/77", hasSource: true, extID: "77", plainPlugin: true},
		progCase{name: "source plugin unregistered", newStatus: "in_progress", extURL: "https://github.com/o/r/issues/77", hasSource: true, extID: "77", noPlugin: true},
	)
}

func goldenProgress(t *testing.T, g *strings.Builder) {
	g.WriteString("== progress comments\n")
	for _, c := range progCases() {
		log := &goldenLog{}
		storage := svctest.NewForwardSyncTestStorage(t)
		registry := session.NewPluginRegistry()
		prog := &gProgress{gPlain: gPlain{id: "g_progress"}, log: log, err: c.err, updatedAt: c.updatedAt}
		pluginID := "g_progress"
		switch {
		case c.noPlugin:
		case c.plainPlugin:
			registry.Register(gPlain{id: pluginID})
		default:
			registry.Register(prog)
		}
		sourceID := ""
		if c.hasSource {
			src, err := storage.CreateItemSource(context.Background(), backlogdto.ItemSourceData{PluginID: pluginID, DisplayName: "s", Config: `{"k":"v"}`, Enabled: true})
			require.NoError(t, err)
			sourceID = src.ID
		}
		if !c.hasSource && !c.noPlugin {
			registry.Register(sourcelessProgress{log: log, err: c.err, updatedAt: c.updatedAt})
		}
		item, err := storage.CreateBacklogItem(context.Background(), backlogdto.BacklogItemData{
			Title: "t", Status: c.newStatus, SourceID: sourceID, ExternalID: c.extID, ExternalURL: c.extURL, ClaimingHostID: c.host,
		})
		require.NoError(t, err)
		update := backlogdto.BacklogItemUpdate{}
		if c.pr != "" {
			pr := c.pr
			update.PrURL = &pr
		}
		if c.lastStatus != "" {
			ls := c.lastStatus
			update.GitHubLastProgressCommentStatus = &ls
		}
		if c.pr != "" || c.lastStatus != "" {
			_, err = storage.UpdateBacklogItem(context.Background(), item.ID, update, nil)
			require.NoError(t, err)
		}
		handleProgressComment(context.Background(), registry, session.NewSyncLoop(storage, registry), storage, item, c.newStatus)
		fmt.Fprintf(g, "case %s\n", c.name)
		flush(g, log)
		observe(t, g, storage, item.ID, "")
	}
	handleProgressComment(context.Background(), nil, nil, nil, nil, "in_progress")
	g.WriteString("nil item: no panic\n")
	handleProgressComment(context.Background(), nil, nil, nil, &backlogdto.BacklogItemData{}, "in_progress")
	g.WriteString("empty id: no panic\n")
	handleForwardSyncCloseWith(context.Background(), nil, nil, nil, nil, sourcelessCloseDeps{})
	handleForwardSyncCloseWith(context.Background(), nil, nil, nil, &backlogdto.BacklogItemData{}, sourcelessCloseDeps{})
	g.WriteString("forward nil/empty item: no panic\n")
}

// sourcelessProgress stands in for the registered github_issues plugin id.
type sourcelessProgress struct {
	log       *goldenLog
	err       error
	updatedAt time.Time
}

func (p sourcelessProgress) PluginID() string { return githubIssuesPluginID }
func (p sourcelessProgress) Fetch(_ context.Context, _ session.PluginConfig, cursor string) ([]session.ExternalItem, string, error) {
	return nil, cursor, nil
}
func (p sourcelessProgress) MapToBacklogItem(_ session.ExternalItem, _ string) backlogdto.BacklogItemData {
	return backlogdto.BacklogItemData{}
}
func (p sourcelessProgress) PostProgressComment(_ context.Context, cfg session.PluginConfig, externalID, body string) (time.Time, error) {
	p.log.add("progress cfg=%s ext=%q body=%q", cfg.Raw, externalID, body)
	return p.updatedAt, p.err
}

func goldenPure(g *strings.Builder) {
	g.WriteString("== comment builders\n")
	for _, host := range []string{"", "h1", "a b"} {
		fmt.Fprintf(g, "closeFor(%q)=%q\n", host, forwardSyncCloseCommentFor(host))
		for _, pr := range []string{"", "https://github.com/o/r/pull/1"} {
			fmt.Fprintf(g, "closeWithPR(%q,%q)=%q\n", host, pr, forwardSyncCloseCommentWithPR(host, pr))
		}
		fmt.Fprintf(g, "started(%q)=%q\n", host, progressStartedComment(host))
	}
	fmt.Fprintf(g, "stateComment=%q\n", fmt.Sprintf(forwardSyncStateComment, "In Review"))
	for _, u := range []string{"", "https://github.com/o/r/pull/5", "https://github.com/o/r/issues/5", "http://github.com/o/r/pull/5", "https://github.com/o/r/pull/x", "https://github.com/o/r/pull/5/files", "https://example.com/o/r/pull/5"} {
		fmt.Fprintf(g, "reviewComment(%q)=%q\n", u, progressReviewComment(u))
	}
	multi := [][]progressPR{
		nil,
		{{URL: "https://github.com/o/r/pull/1"}},
		{{URL: "https://github.com/o/r/pull/1", State: domain.PRStateMerged}, {URL: "https://github.com/o/r/pull/2"}},
		{{URL: "https://github.com/o/r/pull/1", State: domain.PRStateMerged}, {URL: "https://example.com/o/r/pull/2"}},
		{{URL: "https://example.com/o/r/pull/1"}, {URL: "https://github.com/o/r/issues/3"}},
		{{URL: "https://github.com/o/r/pull/1"}, {URL: "https://github.com/o/r/pull/2", State: domain.PRStateMerged}, {URL: "bad"}},
	}
	for i, prs := range multi {
		fmt.Fprintf(g, "reviewComments[%d]=%q\n", i, progressReviewComments(prs))
	}
	for _, last := range []string{"", "in_progress", "review"} {
		for _, st := range []string{"in_progress", "review", "done"} {
			fmt.Fprintf(g, "alreadyPosted(%q,%q)=%v\n", last, st, progressCommentAlreadyPosted(last, st))
		}
	}
	fmt.Fprintf(g, "timeouts forward=%s progress=%s\n", forwardSyncCloseTimeout, progressCommentTimeout)
	fmt.Fprintf(g, "pluginID=%s\n", githubIssuesPluginID)
	reg := session.NewPluginRegistry()
	d := defaultSourcelessCloseDeps(reg)
	fmt.Fprintf(g, "defaultDeps(empty registry): plugin=%v token=%v\n", d.plugin != nil, d.tokenFor != nil)
	d = defaultSourcelessCloseDeps(nil)
	fmt.Fprintf(g, "defaultDeps(nil registry): plugin=%v token=%v\n", d.plugin != nil, d.tokenFor != nil)
	reg.Register(gPlain{id: githubIssuesPluginID})
	d = defaultSourcelessCloseDeps(reg)
	fmt.Fprintf(g, "defaultDeps(non-closer github_issues): plugin=%v\n", d.plugin != nil)
	reg2 := session.NewPluginRegistry()
	reg2.Register(&gCloser{id: githubIssuesPluginID, log: &goldenLog{}})
	d = defaultSourcelessCloseDeps(reg2)
	fmt.Fprintf(g, "defaultDeps(closer github_issues): plugin=%v\n", d.plugin != nil)
}

type subEvent struct {
	name    string
	payload *events.BacklogItemEventPayload
	other   bool
}

func goldenForwardSubscriber(t *testing.T, g *strings.Builder) {
	g.WriteString("== forward subscriber event filtering\n")
	log := &goldenLog{}
	storage := svctest.NewForwardSyncTestStorage(t)
	registry := session.NewPluginRegistry()
	registry.Register(&gCloser{id: "g_plugin", log: log, closeAt: gT1})
	syncLoop := session.NewSyncLoop(storage, registry)
	srcID := svctest.CreateForwardSyncTestSource(t, storage, "g_plugin", true, "lbl")

	mk := func(ext string) *backlogdto.BacklogItemData {
		return svctest.CreateForwardSyncTestItem(t, storage, srcID, ext, nil)
	}
	evs := []subEvent{
		{name: "nil payload"},
		{name: "item updated", payload: &events.BacklogItemEventPayload{Kind: events.BacklogChangeItemUpdated, NewStatus: "done", Item: mk("e1")}},
		{name: "transition to review", payload: &events.BacklogItemEventPayload{Kind: events.BacklogChangeStatusTransition, NewStatus: "review", Item: mk("e2")}},
		{name: "transition to in_progress", payload: &events.BacklogItemEventPayload{Kind: events.BacklogChangeStatusTransition, NewStatus: "in_progress", Item: mk("e3")}},
		{name: "transition to done nil item", payload: &events.BacklogItemEventPayload{Kind: events.BacklogChangeStatusTransition, NewStatus: "done"}},
		{name: "transition to done", payload: &events.BacklogItemEventPayload{Kind: events.BacklogChangeStatusTransition, NewStatus: "done", Item: mk("e4")}},
	}
	bus := events.NewEventBus(100)
	defer bus.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := StartBacklogGitHubForwardSyncSubscriber(ctx, bus, registry, syncLoop, storage)
	for _, e := range evs {
		switch {
		case e.other:
			bus.Publish(events.NewBacklogItemChangedEvent(nil))
		case e.payload == nil:
			bus.Publish(events.NewBacklogItemChangedEvent(nil))
		default:
			bus.Publish(events.NewBacklogItemChangedEvent(e.payload))
		}
	}
	marker := mk("marker")
	bus.Publish(events.NewBacklogItemChangedEvent(&events.BacklogItemEventPayload{Kind: events.BacklogChangeStatusTransition, NewStatus: "done", Item: marker}))
	wait.RequireEventually(t, func() bool {
		got, err := storage.GetBacklogItem(context.Background(), marker.ID)
		return err == nil && got.GitHubSyncedIssueUpdatedAt != nil
	}, 10*time.Second, 5*time.Millisecond, "marker event not processed")
	for _, e := range evs {
		fmt.Fprintf(g, "event %s\n", e.name)
	}
	flush(g, log)
	for _, e := range evs {
		if e.payload != nil && e.payload.Item != nil {
			got, err := storage.GetBacklogItem(context.Background(), e.payload.Item.ID)
			require.NoError(t, err)
			wm := "nil"
			if got.GitHubSyncedIssueUpdatedAt != nil {
				wm = gts(*got.GitHubSyncedIssueUpdatedAt)
			}
			fmt.Fprintf(g, "item %s watermark=%s\n", got.ExternalID, wm)
		}
	}
	cancel()
	<-done
	g.WriteString("subscriber stopped on ctx cancel\n")
	// The missing-dependency probes need a live context: a cancelled one closes done by itself.
	live, stop := context.WithCancel(context.Background())
	defer stop()

	type nilDep struct {
		name  string
		start func() <-chan struct{}
	}
	for _, d := range []nilDep{
		{"nil bus", func() <-chan struct{} {
			return StartBacklogGitHubForwardSyncSubscriber(live, nil, registry, syncLoop, storage)
		}},
		{"nil registry", func() <-chan struct{} {
			return StartBacklogGitHubForwardSyncSubscriber(live, bus, nil, syncLoop, storage)
		}},
		{"nil syncloop", func() <-chan struct{} {
			return StartBacklogGitHubForwardSyncSubscriber(live, bus, registry, nil, storage)
		}},
		{"nil storage", func() <-chan struct{} {
			return StartBacklogGitHubForwardSyncSubscriber(live, bus, registry, syncLoop, nil)
		}},
	} {
		name := d.name
		select {
		case <-d.start():
			fmt.Fprintf(g, "missing dependency %s: done closed\n", name)
		case <-time.After(5 * time.Second):
			fmt.Fprintf(g, "missing dependency %s: done NOT closed\n", name)
		}
	}
}

func goldenProgressSubscriber(t *testing.T, g *strings.Builder) {
	g.WriteString("== progress subscriber event filtering\n")
	log := &goldenLog{}
	storage := svctest.NewForwardSyncTestStorage(t)
	registry := session.NewPluginRegistry()
	registry.Register(&gProgress{gPlain: gPlain{id: "g_progress"}, log: log, updatedAt: gT2})
	syncLoop := session.NewSyncLoop(storage, registry)
	src, err := storage.CreateItemSource(context.Background(), backlogdto.ItemSourceData{PluginID: "g_progress", DisplayName: "s", Config: `{}`, Enabled: true})
	require.NoError(t, err)
	mk := func(ext string) *backlogdto.BacklogItemData {
		item, err := storage.CreateBacklogItem(context.Background(), backlogdto.BacklogItemData{Title: "t", Status: "in_progress", SourceID: src.ID, ExternalID: ext, ExternalURL: "https://github.com/o/r/issues/" + ext})
		require.NoError(t, err)
		pr := "https://github.com/o/r/pull/" + ext
		_, err = storage.UpdateBacklogItem(context.Background(), item.ID, backlogdto.BacklogItemUpdate{PrURL: &pr}, nil)
		require.NoError(t, err)
		return item
	}
	var mu sync.Mutex
	flagReads := 0
	enabled := false
	flag := func() bool { mu.Lock(); defer mu.Unlock(); flagReads++; return enabled }
	reads := func() int { mu.Lock(); defer mu.Unlock(); return flagReads }

	bus := events.NewEventBus(100)
	defer bus.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := StartBacklogGitHubProgressSyncSubscriber(ctx, bus, flag, registry, syncLoop, storage)

	send := func(kind events.BacklogChangeKind, status string, item *backlogdto.BacklogItemData) {
		bus.Publish(events.NewBacklogItemChangedEvent(&events.BacklogItemEventPayload{Kind: kind, NewStatus: status, Item: item}))
	}
	type step struct {
		name   string
		kind   events.BacklogChangeKind
		status string
		ext    string
	}
	steps := []step{
		{"updated in_progress", events.BacklogChangeItemUpdated, "in_progress", "1"},
		{"transition done", events.BacklogChangeStatusTransition, "done", "2"},
		{"transition triage", events.BacklogChangeStatusTransition, "triage", "3"},
	}
	for _, s := range steps {
		send(s.kind, s.status, mk(s.ext))
	}
	bus.Publish(events.NewBacklogItemChangedEvent(nil))
	g.WriteString("flag off, ignored kinds/statuses sent\n")
	flagOffItem := mk("4")
	send(events.BacklogChangeStatusTransition, "in_progress", flagOffItem)
	wait.RequireEventually(t, func() bool { return reads() == 1 }, 10*time.Second, 5*time.Millisecond, "flag read once")
	fmt.Fprintf(g, "flag reads after filtered events + one in_progress: %d\n", reads())
	flush(g, log)

	mu.Lock()
	enabled = true
	mu.Unlock()
	onItem := mk("5")
	send(events.BacklogChangeStatusTransition, "in_progress", onItem)
	reviewItem := mk("6")
	send(events.BacklogChangeStatusTransition, "review", reviewItem)
	wait.RequireEventually(t, func() bool {
		got, err := storage.GetBacklogItem(context.Background(), reviewItem.ID)
		return err == nil && got.GitHubLastProgressCommentStatus == "review"
	}, 10*time.Second, 5*time.Millisecond, "review comment processed")
	g.WriteString("flag on\n")
	flush(g, log)
	for _, it := range []*backlogdto.BacklogItemData{flagOffItem, onItem, reviewItem} {
		got, err := storage.GetBacklogItem(context.Background(), it.ID)
		require.NoError(t, err)
		wm := "nil"
		if got.GitHubSyncedIssueUpdatedAt != nil {
			wm = gts(*got.GitHubSyncedIssueUpdatedAt)
		}
		fmt.Fprintf(g, "item %s lastProgress=%q watermark=%s\n", got.ExternalID, got.GitHubLastProgressCommentStatus, wm)
	}
	cancel()
	<-done
	g.WriteString("subscriber stopped on ctx cancel\n")
	// The missing-dependency probes need a live context: a cancelled one closes done by itself.
	live, stop := context.WithCancel(context.Background())
	defer stop()
	probe := func(name string, ch <-chan struct{}) {
		select {
		case <-ch:
			fmt.Fprintf(g, "missing dependency %s: done closed\n", name)
		case <-time.After(5 * time.Second):
			fmt.Fprintf(g, "missing dependency %s: done NOT closed\n", name)
		}
	}
	probe("nil bus", StartBacklogGitHubProgressSyncSubscriber(live, nil, flag, registry, syncLoop, storage))
	probe("nil flag", StartBacklogGitHubProgressSyncSubscriber(live, bus, nil, registry, syncLoop, storage))
	probe("nil registry", StartBacklogGitHubProgressSyncSubscriber(live, bus, flag, nil, syncLoop, storage))
	probe("nil syncloop", StartBacklogGitHubProgressSyncSubscriber(live, bus, flag, registry, nil, storage))
	probe("nil storage", StartBacklogGitHubProgressSyncSubscriber(live, bus, flag, registry, syncLoop, nil))
}

func TestForwardSyncGolden(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	var g strings.Builder
	goldenPure(&g)
	goldenForward(t, &g)
	goldenSourceless(t, &g)
	goldenProgress(t, &g)
	goldenForwardSubscriber(t, &g)
	goldenProgressSubscriber(t, &g)
	got := g.String()

	if os.Getenv("FORWARDSYNC_GOLDEN_RECORD") == "1" {
		require.NoError(t, os.MkdirAll(filepath.Dir(goldenPath), 0o755))
		require.NoError(t, os.WriteFile(goldenPath, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	require.Equal(t, string(want), got)
}
