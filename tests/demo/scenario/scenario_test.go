package scenario

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

var refA = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

type call struct{ service, method string }

type fakeClient struct {
	mu    sync.Mutex
	calls []call
}

func (f *fakeClient) Call(_ context.Context, service, method string, _, resp any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call{service, method})
	if out, ok := resp.(*struct {
		Item struct {
			ID string `json:"id"`
		} `json:"item"`
	}); ok {
		out.Item.ID = "item-x"
	}
	return nil
}

func TestDefaultScenarioDelegatesToLegacySeeder(t *testing.T) {
	t.Parallel()
	var gotDir string
	var gotCount int
	deps := Deps{WriteDefault: func(dir string, n int) error { gotDir, gotCount = dir, n; return nil }}
	c, err := Compose([]string{DefaultName}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SeedFiles("d", refA); err != nil {
		t.Fatal(err)
	}
	if gotDir != "d" || gotCount != 6 {
		t.Fatalf("legacy seeder got (%q, %d), want (d, 6)", gotDir, gotCount)
	}
	b, _ := c.Bundle(refA)
	if len(b.keys()) != 0 {
		t.Fatalf("default scenario must add no bundle data, got %v", b.keys())
	}
}

func TestScenariosComposeDuplicateIDErrors(t *testing.T) {
	t.Parallel()
	if _, err := Compose([]string{"dashboard", "approvals"}, Deps{}); err != nil {
		t.Fatalf("dashboard,approvals should compose: %v", err)
	}
	_, err := Compose([]string{"backlog", "backlog"}, Deps{})
	if err == nil || strings.Count(err.Error(), `"backlog"`) < 2 {
		t.Fatalf("want duplicate error naming both scenarios, got %v", err)
	}
	if _, err := Compose([]string{"nope"}, Deps{}); err == nil {
		t.Fatal("unknown scenario must error")
	}
}

func TestAllScenariosComposeTogether(t *testing.T) {
	t.Parallel()
	names := Names()
	if _, err := Compose(names, Deps{}); err != nil {
		t.Fatalf("all scenarios must have disjoint ids: %v", err)
	}
}

func TestEveryScenarioNonEmpty(t *testing.T) {
	t.Parallel()
	for _, n := range Names() {
		if n == DefaultName {
			continue
		}
		b, err := BuildBundle(n, refA)
		if err != nil || len(b.keys()) == 0 {
			t.Errorf("%s: err=%v keys=%d", n, err, len(b.keys()))
		}
	}
}

func TestSeedFilesWritesBundleSessions(t *testing.T) {
	t.Parallel()
	var got []SessionRecord
	deps := Deps{WriteSessions: func(_ string, r []SessionRecord) error { got = r; return nil }}
	c, err := Compose([]string{"dashboard"}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SeedFiles(t.TempDir(), refA); err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("want 4 dashboard sessions, got %d", len(got))
	}
	for _, s := range got {
		if s.Status != "active" && s.Status != "paused" {
			t.Errorf("%s: status %q would be reclassified by the poller", s.ID, s.Status)
		}
	}
}

func TestSeedRPCCallsExpectedEndpoints(t *testing.T) {
	t.Parallel()
	c, err := Compose([]string{"backlog", "approvals"}, Deps{WorkDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeClient{}
	if err := c.SeedRPC(context.Background(), fc, refA); err != nil {
		t.Fatal(err)
	}
	count := map[call]int{}
	for _, cl := range fc.calls {
		count[cl]++
	}
	want := map[call]int{
		{"SessionService", "UpdateFeatureFlag"}:           1,
		{"SessionService", "UpsertApprovalRule"}:          3,
		{"BacklogService", "CreateBacklogItem"}:           5,
		{"BacklogService", "TransitionBacklogItemStatus"}: 4,
		{"SessionService", "CreateSession"}:               1,
	}
	if !reflect.DeepEqual(count, want) {
		t.Fatalf("calls = %v, want %v", count, want)
	}
}

func TestTimestampsAreFixedOffsetsFromRef(t *testing.T) {
	t.Parallel()
	refB := refA.Add(37*time.Hour + 5*time.Minute)
	for _, n := range Names() {
		if n == DefaultName {
			continue
		}
		a, _ := BuildBundle(n, refA)
		b, _ := BuildBundle(n, refB)
		ta, tb := collectTimes(reflect.ValueOf(a)), collectTimes(reflect.ValueOf(b))
		if len(ta) != len(tb) {
			t.Fatalf("%s: timestamp count differs", n)
		}
		for i := range ta {
			if ta[i].Sub(refA) != tb[i].Sub(refB) {
				t.Errorf("%s: timestamp %d offset differs: %v vs %v", n, i, ta[i].Sub(refA), tb[i].Sub(refB))
			}
			if !ta[i].Before(refA) {
				t.Errorf("%s: timestamp %d is not before ref", n, i)
			}
		}
	}
}

func collectTimes(v reflect.Value) []time.Time {
	var out []time.Time
	timeType := reflect.TypeOf(time.Time{})
	var walk func(reflect.Value)
	walk = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Struct:
			if v.Type() == timeType {
				out = append(out, v.Interface().(time.Time))
				return
			}
			for i := 0; i < v.NumField(); i++ {
				walk(v.Field(i))
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i))
			}
		}
	}
	walk(v)
	return out
}

func TestRouteFixturesAreMarkedRoute(t *testing.T) {
	t.Parallel()
	for _, n := range Names() {
		if n == DefaultName {
			continue
		}
		b, _ := BuildBundle(n, refA)
		for _, r := range b.Routes {
			if r.Seeded != SeededRoute {
				t.Errorf("%s/%s: seeded = %q, want route", n, r.ID, r.Seeded)
			}
		}
	}
}
