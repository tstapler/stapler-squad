package taggingrules

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

var updateTaggingGolden = flag.Bool("update-taggingrules-golden", false, "rewrite testdata/taggingrules.golden")

var goldenCreatedAt = time.Date(2024, 3, 2, 1, 0, 0, 123456000, time.UTC)

type transcript struct{ b strings.Builder }

func (tr *transcript) line(format string, args ...any) {
	fmt.Fprintf(&tr.b, format+"\n", args...)
}

func goldenErr(err error) string {
	if err == nil {
		return "<nil>"
	}
	s := fmt.Sprintf("%q validation=%t notfound=%t", err.Error(),
		errors.Is(err, ErrTaggingRuleValidation), errors.Is(err, ErrTaggingRuleNotFound))
	var ce *connect.Error
	if errors.As(err, &ce) {
		s += " code=" + ce.Code().String()
	}
	return s
}

// maskSpec replaces a wall-clock CreatedAt (anything but the fixed input) with a marker.
func maskSpec(s TaggingRuleSpec) string {
	if !s.CreatedAt.IsZero() && s.CreatedAt.Year() != goldenCreatedAt.Year() {
		s.CreatedAt = time.Date(1, 1, 1, 0, 0, 1, 0, time.UTC)
	} else {
		s.CreatedAt = s.CreatedAt.UTC()
	}
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func (tr *transcript) specs(label string, specs []TaggingRuleSpec) {
	tr.line("%s: %d specs", label, len(specs))
	for _, s := range specs {
		tr.line("  %s", maskSpec(s))
	}
}

func (tr *transcript) rules(label string, rules []classifier.TaggingRule) {
	tr.line("%s: %d rules", label, len(rules))
	str := func(re *regexp.Regexp) string {
		if re == nil {
			return "-"
		}
		return fmt.Sprintf("%q", re.String())
	}
	for _, r := range rules {
		tr.line("  id=%q name=%q prio=%d enabled=%t source=%q n=%s b=%s p=%s g=%s req=%#v out=%q",
			r.ID, r.Name, r.Priority, r.Enabled, r.Source, str(r.NamePattern), str(r.BranchPattern),
			str(r.PathPattern), str(r.ProgramPattern), r.RequiredTags, r.OutputTag)
	}
}

func (tr *transcript) rows(label string, storage *session.Storage) {
	rows, err := storage.AllTaggingRules(context.Background())
	tr.line("%s: %d rows err=%s", label, len(rows), goldenErr(err))
	for _, r := range rows {
		tr.line("  createdSet=%t updatedSet=%t", !r.CreatedAt.IsZero(), !r.UpdatedAt.IsZero())
		if r.CreatedAt.Year() != goldenCreatedAt.Year() {
			r.CreatedAt = time.Time{}
		} else {
			r.CreatedAt = r.CreatedAt.UTC()
		}
		r.UpdatedAt = time.Time{}
		b, err := json.Marshal(r)
		if err != nil {
			panic(err)
		}
		tr.line("  %s", b)
	}
}

func goldenStorage(t *testing.T) (*session.Storage, *session.EntRepository) {
	t.Helper()
	repo := session.NewTestEntRepository(t)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)
	return storage, repo
}

func goldenSpecs() []TaggingRuleSpec {
	full := TaggingRuleSpec{ID: "u-full", Name: "Full <&> ünï", NamePattern: "^n", BranchPattern: "^b/", PathPattern: "/p/",
		ProgramPattern: "cla.de", RequiredTags: []string{"A", "B"}, OutputTag: "Full", Priority: 10, Enabled: true,
		Source: "user", CreatedAt: goldenCreatedAt}
	updated := full
	updated.Name, updated.Priority, updated.RequiredTags = "Full v2", 99, nil
	return []TaggingRuleSpec{
		{},
		{ID: "x"},
		{OutputTag: "x"},
		{ID: "seed-lookalike", Name: "lookalike", OutputTag: "x", Source: "user", CreatedAt: goldenCreatedAt},
		{ID: classifier.SeedTaggingRules()[0].ID, OutputTag: "x"},
		{ID: "bad-n", OutputTag: "x", NamePattern: "["},
		{ID: "bad-b", OutputTag: "x", BranchPattern: "("},
		{ID: "bad-p", OutputTag: "x", PathPattern: "*"},
		{ID: "bad-g", OutputTag: "x", ProgramPattern: "a{2,1}"},
		{ID: "bad-two", OutputTag: "x", BranchPattern: "(", NamePattern: "["},
		full,
		{ID: "u-min", OutputTag: "Min"},
		{ID: "u-empty-req", Name: "empty req", OutputTag: "E", RequiredTags: []string{}, Priority: -5, Source: "seed", CreatedAt: goldenCreatedAt},
		updated,
		{ID: "u-min", Name: "min v2", OutputTag: "Min2", Enabled: true, CreatedAt: goldenCreatedAt},
	}
}

func goldenStore(t *testing.T, tr *transcript) {
	ctx := context.Background()
	storage, repo := goldenStorage(t)
	store, err := NewTaggingRulesStore(storage)
	tr.line("new store err=%s", goldenErr(err))
	tr.specs("initial", store.All())
	tr.rules("initial", store.ToRules())
	for i, spec := range goldenSpecs() {
		saved, err := store.Upsert(ctx, spec)
		tr.line("upsert[%d] -> %s err=%s", i, maskSpec(saved), goldenErr(err))
	}
	tr.specs("all", store.All())
	tr.rules("compiled", store.ToRules())
	tr.rows("db", storage)
	all := store.All()
	all[0].Name = "mutated copy"
	tr.line("All returns a copy: %t", store.All()[0].Name != "mutated copy")
	store.WatchAndReload(ctx)
	tr.line("delete unknown err=%s", goldenErr(store.Delete(ctx, "nope")))
	tr.line("delete empty err=%s", goldenErr(store.Delete(ctx, "")))
	tr.line("delete u-full err=%s", goldenErr(store.Delete(ctx, "u-full")))
	tr.line("delete u-full again err=%s", goldenErr(store.Delete(ctx, "u-full")))
	tr.specs("after delete", store.All())
	tr.rows("db after delete", storage)

	raw := func(id, branch, path string, prio int) {
		require.NoError(t, storage.UpsertTaggingRule(ctx, domain.TaggingRuleData{RuleID: id, Name: id, BranchPattern: branch,
			PathPattern: path, OutputTag: "R", Priority: prio, Source: "user", CreatedAt: goldenCreatedAt, UpdatedAt: goldenCreatedAt}))
	}
	raw("raw-bad", "(", "", 3)
	raw("raw-ok", "", "^/x", 1)
	reloaded, err := NewTaggingRulesStore(storage)
	tr.line("reload err=%s", goldenErr(err))
	tr.specs("reloaded", reloaded.All())
	tr.rules("reloaded compiled", reloaded.ToRules())

	require.NoError(t, repo.Close())
	saved, err := reloaded.Upsert(ctx, TaggingRuleSpec{ID: "after-close", Name: "a", OutputTag: "x"})
	tr.line("closed upsert -> %s err=%s", maskSpec(saved), goldenErr(err))
	tr.line("closed delete known err=%s", goldenErr(reloaded.Delete(ctx, "raw-ok")))
	tr.line("closed delete unknown err=%s", goldenErr(reloaded.Delete(ctx, "nope")))
	tr.specs("closed all unchanged", reloaded.All())
	_, err = NewTaggingRulesStore(storage)
	tr.line("closed new store err=%s", goldenErr(err))
}

func (tr *transcript) list(label string, svc *TaggingRulesService) {
	got, err := svc.ListTaggingRules(context.Background())
	tr.line("%s: list %d err=%s", label, len(got), goldenErr(err))
	for _, r := range got {
		tr.line("  %s fires=%d", maskSpec(r.TaggingRuleSpec), r.FireCount7d)
	}
}

func goldenService(t *testing.T, tr *transcript) {
	ctx := context.Background()
	storage, repo := goldenStorage(t)
	store, err := NewTaggingRulesStore(storage)
	require.NoError(t, err)
	engine := classifier.NewTaggingEngine()
	svc := NewTaggingRulesService(store, engine, goldenCounter(storage))
	noCounts := NewTaggingRulesService(store, engine, nil)
	tr.list("seed only", svc)
	for i, spec := range goldenSpecs() {
		saved, err := svc.UpsertTaggingRule(ctx, spec)
		tr.line("svc upsert[%d] -> %s err=%s", i, maskSpec(saved), goldenErr(err))
	}
	tr.rules("engine", engine.Rules())
	seedID := classifier.SeedTaggingRules()[0].ID
	fire := func(id string, age time.Duration) {
		require.NoError(t, storage.RecordTaggingRuleFire(ctx, id, time.Now().Add(-age)))
	}
	for _, id := range []string{"u-full", "u-full", "u-min", seedID} {
		fire(id, 0)
	}
	fire("u-min", 6*24*time.Hour)
	fire("u-min", 8*24*time.Hour)
	tr.list("with counts", svc)
	tr.list("nil counter", noCounts)
	tr.line("svc delete unknown err=%s", goldenErr(svc.DeleteTaggingRule(ctx, "nope")))
	tr.line("svc delete u-full err=%s", goldenErr(svc.DeleteTaggingRule(ctx, "u-full")))
	tr.rules("engine after delete", engine.Rules())
	require.NoError(t, repo.Close())
	_, err = svc.UpsertTaggingRule(ctx, TaggingRuleSpec{ID: "after-close", Name: "a", OutputTag: "x"})
	tr.line("closed svc upsert err=%s", goldenErr(err))
	tr.line("closed svc delete err=%s", goldenErr(svc.DeleteTaggingRule(ctx, "u-min")))
	tr.list("closed (count error)", svc)
	tr.rules("engine after closed", engine.Rules())
}

func (tr *transcript) wire(label string, msg proto.Message, err error) {
	if err != nil {
		tr.line("%s: err=%s", label, goldenErr(err))
		return
	}
	b, merr := proto.MarshalOptions{Deterministic: true}.Marshal(msg)
	if merr != nil {
		panic(merr)
	}
	tr.line("%s: %s", label, hex.EncodeToString(b))
}

var goldenUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// maskRule removes the generated id and wall-clock created_at before the bytes are recorded.
func (tr *transcript) maskRule(p *sessionv1.TaggingRuleProto) {
	if goldenUUID.MatchString(p.GetId()) {
		tr.line("  id=<uuid>")
		p.Id = "<uuid>"
	}
	if ts := p.GetCreatedAt(); ts != nil && ts.AsTime().Year() != goldenCreatedAt.Year() {
		tr.line("  created_at=<now>")
		p.CreatedAt = nil
	}
}

type goldenRPCClient struct {
	t   *testing.T
	tr  *transcript
	svc *TaggingRulesService
}

func (c goldenRPCClient) list(label string) {
	resp, err := c.svc.ListTaggingRulesRPC(context.Background(), connect.NewRequest(&sessionv1.ListTaggingRulesRequest{}))
	if err != nil {
		c.tr.wire(label, nil, err)
		return
	}
	for _, p := range resp.Msg.GetRules() {
		c.tr.maskRule(p)
	}
	c.tr.wire(label, resp.Msg, nil)
}

func (c goldenRPCClient) upsert(label string, r *sessionv1.TaggingRuleProto) {
	resp, err := c.svc.UpsertTaggingRuleRPC(context.Background(), connect.NewRequest(&sessionv1.UpsertTaggingRuleRequest{Rule: r}))
	if err != nil {
		c.tr.wire(label, nil, err)
		return
	}
	c.tr.maskRule(resp.Msg.GetRule())
	c.tr.wire(label, resp.Msg, nil)
}

func (c goldenRPCClient) del(label, id string) {
	resp, err := c.svc.DeleteTaggingRuleRPC(context.Background(), connect.NewRequest(&sessionv1.DeleteTaggingRuleRequest{Id: id}))
	if err != nil {
		c.tr.wire(label, nil, err)
		return
	}
	c.tr.wire(label, resp.Msg, nil)
}

func goldenRPC(t *testing.T, tr *transcript) {
	ctx := context.Background()
	storage, repo := goldenStorage(t)
	store, err := NewTaggingRulesStore(storage)
	require.NoError(t, err)
	c := goldenRPCClient{t, tr, NewTaggingRulesService(store, classifier.NewTaggingEngine(), goldenCounter(storage))}
	c.list("rpc list seed")
	c.upsert("rpc nil rule", nil)
	c.upsert("rpc no output tag", &sessionv1.TaggingRuleProto{Id: "a"})
	c.upsert("rpc bad regex", &sessionv1.TaggingRuleProto{Id: "a", OutputTag: "o", BranchPattern: "("})
	c.upsert("rpc seed id", &sessionv1.TaggingRuleProto{Id: classifier.SeedTaggingRules()[0].ID, OutputTag: "o"})
	full := taggingRuleSpecToProto(goldenSpecs()[10], 42)
	full.Source = "seed"
	c.upsert("rpc update with id", full)
	c.upsert("rpc create", &sessionv1.TaggingRuleProto{Name: "gen", OutputTag: "gen", Enabled: true, Priority: -1})
	require.NoError(t, storage.RecordTaggingRuleFire(ctx, "u-full", time.Now()))
	c.list("rpc list")
	c.del("rpc delete empty", "")
	c.del("rpc delete unknown", "nope")
	c.del("rpc delete ok", "u-full")
	require.NoError(t, repo.Close())
	c.upsert("rpc closed upsert", &sessionv1.TaggingRuleProto{Id: "z", Name: "z", OutputTag: "o"})
	left := store.All()
	require.Len(t, left, 1)
	_, err = c.svc.DeleteTaggingRuleRPC(ctx, connect.NewRequest(&sessionv1.DeleteTaggingRuleRequest{Id: left[0].ID}))
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	tr.line("rpc closed delete: code=%s err=%q", ce.Code(), ce.Message())
	c.list("rpc closed list")
}

// TestTaggingRulesGolden pins persisted rows, compiled rules, the CRUD/error matrix and RPC
// response bytes to the transcript recorded from the pre-move code in server/services.
func TestTaggingRulesGolden(t *testing.T) {
	t.Parallel()
	tr := &transcript{}
	goldenStore(t, tr)
	goldenService(t, tr)
	goldenRPC(t, tr)

	path := filepath.Join("testdata", "taggingrules.golden")
	if *updateTaggingGolden {
		require.NoError(t, os.MkdirAll("testdata", 0o750))
		require.NoError(t, os.WriteFile(path, []byte(tr.b.String()), 0o600))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), tr.b.String())
}

// goldenCounter is the fire-count port the transcript uses; the base recording passed the
// pass-through *services.AnalyticsStore over the same storage.
func goldenCounter(s *session.Storage) FireCounter { return s }
