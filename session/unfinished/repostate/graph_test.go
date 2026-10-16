package repostate

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/go-git/go-git/v5/plumbing"
	"golang.org/x/sys/cpu"
)

const g7BytesPerRow = 120

func TestGraph_ShouldFillOnlyRequestedRow_WhenLazyDAG(t *testing.T) {
	// A<-B<-C and D with parents (B, C).
	a, b, c, dd := testHash(1, 0), testHash(1, 1), testHash(1, 2), testHash(1, 3)
	d := &dag{parents: map[plumbing.Hash][]plumbing.Hash{a: nil, b: {a}, c: {b}, dd: {b, c}}}
	g := NewGraph(Options{}).Load()
	id := mustIntern(t, g, dd)

	ids, st := mustFill(t, g, id, &mapSource{d: d}, 0)
	if st != Filled || !equalHashes(hashesOf(g, ids), []plumbing.Hash{b, c}) {
		t.Fatalf("Fill = %v %v, want Filled [B C]", st, hashesOf(g, ids))
	}
	if g.Len() != 3 {
		t.Errorf("Len = %d, want 3 (D, B, C interned)", g.Len())
	}
	for _, h := range []plumbing.Hash{b, c} {
		pid, ok := g.Lookup(h)
		if !ok || g.Row(pid, 0) != Pending {
			t.Errorf("%s: ok=%v state=%v, want interned Pending", h, ok, g.Row(pid, 0))
		}
	}
	if _, ok := g.Lookup(a); ok {
		t.Error("A interned, want not (no eager closure)")
	}
}

func TestGraph_ShouldKeepSegmentsFixed_WhenTenThousandRowsAppendedUnderReaders(t *testing.T) {
	gr := NewGraph(Options{MaxGraphCommits: 10000})
	g := gr.Load()
	if len(g.dir) != 3 {
		t.Fatalf("directory length = %d, want ceil(10000/4096) = 3", len(g.dir))
	}
	first := mustIntern(t, g, testHash(2, 0))
	segBefore := g.dir[0].Load()
	hashBefore := g.Hash(first)

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for r := 0; r < 8; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = g.Row(first, 0)
					if g.Hash(first) != hashBefore {
						t.Error("row 0 hash changed under reader")
						return
					}
				}
			}
		}()
	}
	for i := 1; i < 10000; i++ {
		mustIntern(t, g, testHash(2, i))
	}
	close(stop)
	wg.Wait()

	if g.Len() != 10000 || len(g.dir) != 3 {
		t.Errorf("Len=%d dir=%d, want 10000 and 3", g.Len(), len(g.dir))
	}
	if g.dir[0].Load() != segBefore {
		t.Error("segment 0 moved after growth")
	}
	assertPointerFree(t, reflect.TypeOf(segment{}))
	assertPointerFree(t, reflect.TypeOf(sideChunk{}))
}

func assertPointerFree(t *testing.T, typ reflect.Type) {
	t.Helper()
	switch typ.Kind() {
	case reflect.Array:
		assertPointerFree(t, typ.Elem())
	case reflect.Struct:
		for i := 0; i < typ.NumField(); i++ {
			assertPointerFree(t, typ.Field(i).Type)
		}
	case reflect.Uint8, reflect.Uint32, reflect.Uint64, reflect.Int32, reflect.Int64:
	default:
		t.Errorf("%v contains pointer-bearing kind %v", typ, typ.Kind())
	}
}

// The paint-down merge base (the only walk, Q3) orders its queue by committer
// time, so the segment carries a `when` column; there is still no generation
// column (parents-first order cannot be guaranteed by lazy fill).
func TestGraphColumns_ShouldBeHashParentsWhenState_WithNoGenerationColumn(t *testing.T) {
	typ := reflect.TypeOf(segment{})
	got := map[string]bool{}
	for i := 0; i < typ.NumField(); i++ {
		got[typ.Field(i).Name] = true
	}
	want := map[string]bool{"hash": true, "parents": true, "when": true, "state": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("segment columns = %v, want exactly %v", got, want)
	}
}

// counterSource gates Parents on a channel pair so a test can observe a row
// mid-fill (Filling) deterministically.
func TestGraphRowStates_ShouldAcceptOnlyListedTransitions_WhenTransitionTableDriven(t *testing.T) {
	d := newDAG(3, 4)
	h := d.hashes[3]
	const ord1, ord2 = StampOrd(5), StampOrd(6)

	type step struct {
		name   string
		ord    StampOrd
		source func(g *Generation, id uint32, seen *RowState) *mapSource
		want   RowState // state after the step
		err    bool
	}
	observe := func(trunc bool, fail error) func(*Generation, uint32, *RowState) *mapSource {
		return func(g *Generation, id uint32, seen *RowState) *mapSource {
			s := &mapSource{d: d, onParents: func(context.Context, plumbing.Hash) error {
				*seen = RowState(g.stateWord(id).Load() & 3) // state inside the fill
				return fail
			}}
			if trunc {
				s.truncated = func(plumbing.Hash) bool { return true }
			}
			return s
		}
	}
	steps := []step{
		{"Pending->Filling->Truncated", ord1, observe(true, nil), Truncated, false},
		{"Truncated(current) is terminal for same ordinal", ord1, observe(false, nil), Truncated, false},
		{"Truncated(stale)->Filling->Pending on error", ord2, observe(false, errors.New("boom")), Pending, true},
		{"Pending->Filling->Filled", ord2, observe(false, nil), Filled, false},
		{"Filled is terminal", ord2, observe(true, nil), Filled, false},
	}
	g := NewGraph(Options{}).Load()
	id := mustIntern(t, g, h)
	for _, s := range steps {
		var inside RowState = 99
		src := s.source(g, id, &inside)
		before := src.calls.Load()
		_, _, err := g.Fill(context.Background(), id, src, s.ord, nil)
		if (err != nil) != s.err {
			t.Fatalf("%s: err=%v", s.name, err)
		}
		if got, _ := unpackState(g.stateWord(id).Load()); got != s.want {
			t.Errorf("%s: state = %v, want %v", s.name, got, s.want)
		}
		if called := src.calls.Load() != before; called && inside != Filling {
			t.Errorf("%s: state during fill = %v, want Filling", s.name, inside)
		}
	}
	if g.stateWord(id).Load()>>stateBits != 0 {
		t.Error("Filled row kept a stamp ordinal")
	}
	st := g.stats
	if st.reprobe.V.Load() != 1 || st.rollback.V.Load() != 1 {
		t.Errorf("reprobe=%d rollback=%d, want 1 and 1", st.reprobe.V.Load(), st.rollback.V.Load())
	}
}

func TestGraph_ShouldRollBackToPending_WhenFillExitsWithoutPublishing(t *testing.T) {
	d := newDAG(4, 3)
	h := d.hashes[2]
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	closed := errors.New("storer closed")
	cases := []struct {
		name string
		fill func(g *Generation, id uint32, src *mapSource) error
		hook func(context.Context, plumbing.Hash) error
	}{
		{"error", nil, func(context.Context, plumbing.Hash) error { return errors.New("read failed") }},
		{"cancel", nil, func(c context.Context, _ plumbing.Hash) error { return c.Err() }},
		{"closed storer", nil, func(context.Context, plumbing.Hash) error { return closed }},
		{"panic", nil, func(context.Context, plumbing.Hash) error { panic("boom") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gr := NewGraph(Options{})
			g := gr.Load()
			id := mustIntern(t, g, h)
			bad := &mapSource{d: d, onParents: tc.hook}
			func() {
				defer func() { _ = recover() }()
				_, _, err := g.Fill(ctx, id, bad, 0, nil)
				if err == nil {
					t.Error("failing fill returned nil error")
				}
			}()
			if g.Row(id, 0) != Pending {
				t.Fatalf("state after failed fill = %v, want Pending", g.Row(id, 0))
			}
			if gr.Stats().Rollback != 1 {
				t.Errorf("Rollback = %d, want 1", gr.Stats().Rollback)
			}
			ids, st := mustFill(t, g, id, &mapSource{d: d}, 0)
			if st != Filled || !equalHashes(hashesOf(g, ids), d.parents[h]) {
				t.Errorf("second fill = %v %v, want Filled with parents", st, hashesOf(g, ids))
			}
		})
	}
}

func TestGraph_ShouldNotStrandWaiter_WhenHolderFailsMidFill(t *testing.T) {
	d := newDAG(5, 3)
	h := d.hashes[2]
	g := NewGraph(Options{ClaimWait: time.Millisecond}).Load() // the stuck holder never wakes the waiter
	id := mustIntern(t, g, h)

	inside, release := make(chan struct{}), make(chan struct{})
	holder := &mapSource{d: d, onParents: func(context.Context, plumbing.Hash) error {
		close(inside)
		<-release
		return errors.New("holder fails")
	}}
	holderErr := make(chan error, 1)
	go func() {
		_, _, err := g.Fill(context.Background(), id, holder, 0, nil)
		holderErr <- err
	}()
	<-inside

	type res struct {
		ids []uint32
		st  RowState
		err error
	}
	waiter := make(chan res, 1)
	go func() {
		ids, st, err := g.Fill(context.Background(), id, &mapSource{d: d}, 0, nil)
		waiter <- res{ids, st, err}
	}()
	// The waiter waits at most ClaimWait, so it may finish (by local decode)
	// before or after the holder rolls back; either way it must return.
	r := <-waiter
	close(release)
	if err := <-holderErr; err == nil {
		t.Error("holder fill succeeded, want its error")
	}
	if r.err != nil || r.st != Filled || !equalHashes(hashesOf(g, r.ids), d.parents[h]) {
		t.Errorf("waiter = %v %v %v, want Filled with parents", r.st, hashesOf(g, r.ids), r.err)
	}
	if _, st := mustFill(t, g, id, &mapSource{d: d}, 0); st != Filled || g.Row(id, 0) != Filled {
		t.Errorf("row not fillable after holder failure: %v", g.Row(id, 0))
	}
}

func TestGraph_ShouldReprobeBoundaryRow_WhenDeepenedBetweenPhaseOneAndPublish(t *testing.T) {
	d := newDAG(6, 3)
	h := d.hashes[2]
	gr := NewGraph(Options{})
	g := gr.Load()
	id := mustIntern(t, g, h)

	ord1 := g.BumpStamp()
	var ord2 StampOrd
	// The deepen lands after this walk's phase 1 captured ord1 and before it
	// publishes; the walk still observed the pre-deepen boundary.
	first := &mapSource{d: d, truncated: func(plumbing.Hash) bool { return true },
		onParents: func(context.Context, plumbing.Hash) error {
			ord2 = g.BumpStamp()
			return nil
		}}
	if _, st := mustFill(t, g, id, first, ord1); st != Truncated {
		t.Fatalf("first fill = %v, want Truncated", st)
	}
	if g.Row(id, ord1) != Truncated {
		t.Error("row stamped with ord1 should read Truncated for an ord1 caller")
	}
	if g.Row(id, ord2) != Pending {
		t.Errorf("row under ord2 = %v, want stale Truncated read as Pending", g.Row(id, ord2))
	}
	ids, st := mustFill(t, g, id, &mapSource{d: d}, ord2)
	if st != Filled || !equalHashes(hashesOf(g, ids), d.parents[h]) {
		t.Errorf("re-probe = %v %v, want Filled with deeper parents", st, hashesOf(g, ids))
	}
	if gr.Stats().Reprobe != 1 {
		t.Errorf("Reprobe = %d, want 1", gr.Stats().Reprobe)
	}
}

func TestGraph_ShouldWritePayloadExactlyOnce_When8GoroutinesRaceSameThousandRows(t *testing.T) {
	d := newDAG(7, 1000)
	var writes [1100]atomic.Int32
	var after atomic.Int32
	var gen *Generation
	hooks := graphHooks{PayloadWritten: func(id uint32) {
		if gen != nil && RowState(gen.stateWord(id).Load()&3) == Filled {
			after.Add(1)
		}
		writes[id].Add(1)
	}}
	gr := newGraph(Options{MaxGraphCommits: 1100}, hooks)
	gen = gr.Load()
	src := &mapSource{d: d}
	ids := make([]uint32, len(d.hashes))
	for i, h := range d.hashes {
		ids[i] = mustIntern(t, gen, h)
	}
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, id := range ids {
				if _, _, err := gen.Fill(context.Background(), id, src, 0, nil); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	for i := range d.hashes {
		if n := writes[ids[i]].Load(); n != 1 {
			t.Fatalf("row %d payload written %d times, want 1", ids[i], n)
		}
	}
	if after.Load() != 0 {
		t.Errorf("%d payload writes after Filled", after.Load())
	}
}

func TestGraph_ShouldServeStableLookups_WhenWriterFillsAndEightReadersRead(t *testing.T) {
	d := newDAG(8, 10000)
	gr := NewGraph(Options{MaxGraphCommits: 12000})
	g := gr.Load()
	src := &mapSource{d: d}
	var wg sync.WaitGroup
	done := make(chan struct{})
	for r := 0; r < 8; r++ {
		wg.Add(1)
		go func(r int) {
			defer wg.Done()
			var buf []uint32
			for i := r; ; i = (i + 8) % len(d.hashes) {
				select {
				case <-done:
					return
				default:
				}
				id, ok := g.Lookup(d.hashes[i])
				if !ok {
					continue
				}
				var st RowState
				if buf, st = g.AppendParents(buf[:0], id); st == Filled && !equalHashes(hashesOf(g, buf), d.parents[d.hashes[i]]) {
					t.Errorf("row %d parents unstable", id)
					return
				}
			}
		}(r)
	}
	for _, h := range d.hashes {
		id := mustIntern(t, g, h)
		mustFill(t, g, id, src, 0)
	}
	close(done)
	wg.Wait()
	seen := map[uint32]bool{}
	for _, h := range d.hashes {
		id, _ := g.Lookup(h)
		if seen[id] || int(id) >= g.Len() {
			t.Fatalf("id %d duplicated or beyond Len %d", id, g.Len())
		}
		seen[id] = true
	}
}

func TestGraph_ShouldKeepFilledRowChecksumStable_When5000RowsAppended(t *testing.T) {
	d := newDAG(9, 8000)
	g := NewGraph(Options{}).Load()
	src := &mapSource{d: d}
	sum := func(upto int) uint64 {
		var s uint64 = 14695981039346656037
		for i := 0; i < upto; i++ {
			id, _ := g.Lookup(d.hashes[i])
			ps, _ := g.AppendParents(nil, id)
			s = (s ^ uint64(id)) * 1099511628211
			for _, p := range ps {
				s = (s ^ uint64(p)) * 1099511628211
			}
		}
		return s
	}
	for i := 0; i < 3000; i++ {
		mustFill(t, g, mustIntern(t, g, d.hashes[i]), src, 0)
	}
	before := sum(3000)
	for i := 3000; i < 8000; i++ {
		mustFill(t, g, mustIntern(t, g, d.hashes[i]), src, 0)
	}
	if after := sum(3000); after != before {
		t.Error("checksum of Filled rows changed after 5000 more rows")
	}
}

func TestGraph_ShouldReturnErrGraphFullAndNeverExceedCap_When8GoroutinesAtCapMinus1CapCapPlusN(t *testing.T) {
	const capRows = 100
	for _, pre := range []int{capRows - 1, capRows, 0} {
		gr := NewGraph(Options{MaxGraphCommits: capRows})
		g := gr.Load()
		for i := 0; i < pre; i++ {
			mustIntern(t, g, testHash(10, i))
		}
		var wg sync.WaitGroup
		var ok, full atomic.Int64
		for w := 0; w < 8; w++ {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				for i := 0; i < capRows; i++ {
					_, err := g.Intern(testHash(10, 1000+w*capRows+i))
					switch {
					case err == nil:
						ok.Add(1)
					case errors.Is(err, ErrGraphFull):
						full.Add(1)
					default:
						t.Error(err)
					}
				}
			}(w)
		}
		wg.Wait()
		if g.Len() != capRows || int(ok.Load())+pre != capRows {
			t.Errorf("pre=%d: Len=%d ok=%d, want exactly the cap %d", pre, g.Len(), ok.Load(), capRows)
		}
		if full.Load() != 8*capRows-ok.Load() || gr.Stats().GraphFull != uint64(full.Load()) {
			t.Errorf("pre=%d: full=%d counter=%d", pre, full.Load(), gr.Stats().GraphFull)
		}
		if g.next.V.Load() != capRows {
			t.Errorf("next=%d overshot cap", g.next.V.Load())
		}
	}
}

func TestGraph_ShouldReturnErrGraphFull_WhenFillNeedsParentRowsBeyondCap(t *testing.T) {
	d := newDAG(11, 50)
	gr := NewGraph(Options{MaxGraphCommits: 2})
	g := gr.Load()
	var h plumbing.Hash
	for _, c := range d.hashes {
		if len(d.parents[c]) >= 2 {
			h = c
			break
		}
	}
	id := mustIntern(t, g, h)
	_, _, err := g.Fill(context.Background(), id, &mapSource{d: d}, 0, nil)
	if len(d.parents[h]) > 1 && !errors.Is(err, ErrGraphFull) {
		t.Fatalf("err = %v, want ErrGraphFull", err)
	}
	if g.Row(id, 0) != Pending {
		t.Errorf("row = %v after ErrGraphFull, want rolled back to Pending", g.Row(id, 0))
	}
}

func TestGraphBytes_ShouldBeUnder120BPerRow_When50000Rows(t *testing.T) {
	g := NewGraph(Options{MaxGraphCommits: 60000})
	gen := g.Load()
	for i := 0; i < 50000; i++ {
		mustIntern(t, gen, testHash(12, i))
	}
	perRow := float64(g.Bytes()) / 50000
	t.Logf("G7 measured Bytes()/row = %.1f B (limit %d)", perRow, g7BytesPerRow)
	if perRow > g7BytesPerRow {
		t.Errorf("Bytes/row = %.1f, want <= %d", perRow, g7BytesPerRow)
	}
}

func TestGraphBytes_ShouldMatchHeapDelta_WhenSeeded50000CommitDAG(t *testing.T) {
	d := newDAG(13, 50000)
	src := &mapSource{d: d}
	heap := func() uint64 {
		runtime.GC()
		runtime.GC()
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return m.HeapAlloc
	}
	before := heap()
	gr := NewGraph(Options{MaxGraphCommits: 60000})
	g := gr.Load()
	for _, h := range d.hashes {
		mustFill(t, g, mustIntern(t, g, h), src, 0)
	}
	after := heap()
	runtime.KeepAlive(gr)
	runtime.KeepAlive(d)
	runtime.KeepAlive(src)
	delta, est := float64(after-before), float64(gr.Bytes())
	t.Logf("HeapAlloc delta = %.0f B, Bytes() = %.0f B (ratio %.2f)", delta, est, est/delta)
	if est < delta*0.75 || est > delta*1.25 {
		t.Errorf("Bytes() = %.0f, want within +/-25%% of heap delta %.0f", est, delta)
	}
}

func TestGraph_ShouldStayWithinBudget_WhenChurnedHistory20RoundsOfAbandonedTips(t *testing.T) {
	const capRows = 4000
	gr := NewGraph(Options{MaxGraphCommits: capRows})
	g := gr.Load()
	ceiling := int64(capRows) * g7BytesPerRow
	for round := 0; round < 20; round++ {
		d := newDAG(int64(100+round), 300)
		src := &mapSource{d: d}
		for _, h := range d.hashes {
			id, err := g.Intern(h)
			if err != nil {
				break
			}
			if _, _, err := g.Fill(context.Background(), id, src, 0, nil); err != nil && !errors.Is(err, ErrGraphFull) {
				t.Fatal(err)
			}
		}
		if b := g.Bytes(); b > ceiling {
			t.Fatalf("round %d: Bytes()=%d exceeds MaxGraphCommits x G7 = %d", round, b, ceiling)
		}
	}
	if g.Len() > capRows {
		t.Errorf("Len=%d exceeds cap", g.Len())
	}

	// A rebuild after a drop decodes no more than the original walk did.
	d := newDAG(500, 300)
	rb := NewGraph(Options{MaxGraphCommits: capRows})
	count := func(gen *Generation) int64 {
		src := &mapSource{d: d}
		for _, h := range d.hashes {
			mustFill(t, gen, mustIntern(t, gen, h), src, 0)
		}
		return src.calls.Load()
	}
	old := count(rb.Load())
	rb.Drop()
	if rebuilt := count(rb.Load()); rebuilt > old {
		t.Errorf("rebuild decoded %d, want <= %d", rebuilt, old)
	}
}

func TestGraphHotAtomics_ShouldBeCachelinePadded_WhenLayoutInspected(t *testing.T) {
	line := unsafe.Sizeof(cpu.CacheLinePad{})
	check := func(typ reflect.Type) {
		var offs []uintptr
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.Type.Kind() == reflect.Struct && f.Type.NumField() == 3 && f.Type.Field(1).Name == "V" {
				if f.Type.Field(0).Type.Size() != line || f.Type.Field(2).Type.Size() != line {
					t.Errorf("%s.%s is not framed by cache-line pads", typ.Name(), f.Name)
				}
				offs = append(offs, f.Offset)
			}
		}
		if len(offs) < 3 {
			t.Fatalf("%s: found %d padded atomics", typ.Name(), len(offs))
		}
		for i := 1; i < len(offs); i++ {
			if offs[i]-offs[i-1] < 2*line {
				t.Errorf("%s: hot atomics %d and %d closer than two lines", typ.Name(), i-1, i)
			}
		}
	}
	check(reflect.TypeOf(Generation{}))
	check(reflect.TypeOf(graphStats{}))
}

func TestGenerationSwap_ShouldBumpEpochAndNeverMixIds_WhenGraphDroppedMidOperation(t *testing.T) {
	d := newDAG(14, 20)
	gr := NewGraph(Options{})
	old := gr.Load()
	idA := mustIntern(t, old, d.hashes[0])
	src := &mapSource{d: d, onParents: func(context.Context, plumbing.Hash) error {
		gr.Drop() // swap lands mid-operation
		return nil
	}}
	id := mustIntern(t, old, d.hashes[19])
	ids, st := mustFill(t, old, id, src, 0)

	fresh := gr.Load()
	if fresh == old || fresh.Epoch() != old.Epoch()+1 {
		t.Fatalf("epoch %d -> %d, want +1 and a new generation", old.Epoch(), fresh.Epoch())
	}
	if st != Filled || !equalHashes(hashesOf(old, ids), d.parents[d.hashes[19]]) {
		t.Errorf("operation on the old generation = %v %v, want consistent old answer", st, hashesOf(old, ids))
	}
	if old.Hash(idA) != d.hashes[0] || fresh.Len() != 0 {
		t.Error("old generation mutated or new generation not empty")
	}
	// Re-ingest in a different order: ids differ for at least one hash.
	for i := len(d.hashes) - 1; i >= 0; i-- {
		mustIntern(t, fresh, d.hashes[i])
	}
	for i, h := range d.hashes {
		oid := mustIntern(t, old, h)
		nid, _ := fresh.Lookup(h)
		if oid != nid {
			return
		}
		_ = i
	}
	t.Error("reversed re-ingest assigned identical ids everywhere")
}

func TestRepoState_ShouldPruneAndDropGraphThroughProductHooks_WhenRegistered(t *testing.T) {
	d := newDAG(15, 100)
	gr := NewGraph(Options{})
	s := NewRepoState(Options{}, 1)
	if !s.Register(TierGraph, gr) {
		t.Fatal("Register refused")
	}
	g := gr.Load()
	for _, h := range d.hashes {
		mustFill(t, g, mustIntern(t, g, h), &mapSource{d: d}, 0)
	}
	if s.Bytes() != gr.Bytes() || s.Bytes() == 0 {
		t.Fatalf("RepoState.Bytes=%d graph=%d", s.Bytes(), gr.Bytes())
	}
	s.Prune(false)
	if gr.Load().Epoch() != 2 || gr.Bytes() >= g.Bytes() {
		t.Errorf("Prune over budget did not swap generation: epoch=%d", gr.Load().Epoch())
	}
	s.DropAll()
	if gr.Load().Epoch() != 3 {
		t.Errorf("DropAll epoch = %d, want 3", gr.Load().Epoch())
	}
}

func TestGraph_ShouldRoundTripOctopusParents_WhenSideArenaChunkBoundaryCrossed(t *testing.T) {
	g := NewGraph(Options{MaxGraphCommits: 2000}).Load()
	parentsOf := map[plumbing.Hash][]plumbing.Hash{}
	var hs []plumbing.Hash
	for i := 0; i < 1500; i++ { // 1500 x 5-word entries cross a 4096-word chunk
		h := testHash(16, i)
		hs = append(hs, h)
		parentsOf[h] = []plumbing.Hash{testHash(17, i%7), testHash(18, i%7), testHash(19, i%7), testHash(20, i%7)}
	}
	src := &mapSource{d: &dag{parents: parentsOf}}
	for _, h := range hs {
		ids, st := mustFill(t, g, mustIntern(t, g, h), src, 0)
		if st != Filled || !equalHashes(hashesOf(g, ids), parentsOf[h]) {
			t.Fatalf("octopus round trip failed for %s", h)
		}
		again, _ := g.AppendParents(nil, mustIntern(t, g, h))
		if !equalHashes(hashesOf(g, again), parentsOf[h]) {
			t.Fatal("AppendParents differs from Fill result")
		}
	}
	if g.sideChunks.Load() < 2 {
		t.Errorf("sideChunks = %d, want the arena to span chunks", g.sideChunks.Load())
	}
}
