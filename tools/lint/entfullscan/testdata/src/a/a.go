// Package a contains test fixtures for the entfullscan analyzer. FakeQuery
// mimics the shape of an ent-generated query builder (a type named *Query
// with a Where and an All method) without depending on real generated code.
package a

import "context"

type FakeQuery struct{}

func (q *FakeQuery) Where(pred string) *FakeQuery           { return q }
func (q *FakeQuery) Order(field string) *FakeQuery          { return q }
func (q *FakeQuery) Limit(n int) *FakeQuery                 { return q }
func (q *FakeQuery) All(ctx context.Context) ([]int, error) { return nil, nil }

func newQuery() *FakeQuery { return &FakeQuery{} }

func applyOptions(q *FakeQuery, opt bool) *FakeQuery { return q }

// BAD1: no Where anywhere — a genuine unfiltered full-table scan.
func bad1(ctx context.Context) {
	q := newQuery()
	_, _ = q.All(ctx) // want `ent query \.All\(ctx\) with no \.Where`
}

// BAD2: Order/Limit but still no Where.
func bad2(ctx context.Context) {
	_, _ = newQuery().Order("created_at").Limit(100).All(ctx) // want `ent query \.All\(ctx\) with no \.Where`
}

// GOOD1: Where directly in the same chain.
func good1(ctx context.Context) {
	_, _ = newQuery().Where("status = 1").All(ctx)
}

// GOOD2: Where nested inside an argument to a helper (applyLoadOptions shape).
func good2(ctx context.Context) {
	_, _ = applyOptions(newQuery().Where("status = 1"), true).All(ctx)
}

// GOOD3: Where applied to the query variable in an earlier statement, not in
// the same expression as .All(ctx) — the cross-statement case.
func good3(ctx context.Context) {
	q := newQuery()
	q = q.Where("status = 1")
	_, _ = q.All(ctx)
}

// GOOD4: Where applied conditionally in an earlier statement.
func good4(ctx context.Context, filtered bool) {
	q := newQuery()
	if filtered {
		q = q.Where("status = 1")
	}
	_, _ = q.All(ctx)
}

// GOOD5: nolint comment on the same line — a real "list everything" endpoint.
func good5(ctx context.Context) {
	q := newQuery()
	_, _ = q.All(ctx) //nolint:entfullscan small, bounded-by-nature table; intentionally returns every row
}

// GOOD6: nolint comment on the preceding line.
func good6(ctx context.Context) {
	q := newQuery()
	//nolint:entfullscan one-time migration backfill; must touch every row
	_, _ = q.All(ctx)
}

// GOOD7: Where applied inside a for loop — a loop-nested variant of the
// cross-statement case (GOOD3/GOOD4).
func good7(ctx context.Context, filters []string) {
	q := newQuery()
	for _, f := range filters {
		q = q.Where(f)
	}
	_, _ = q.All(ctx)
}

// BAD3: a shadowed inner q (declared via := inside a nested block) gets a
// Where, but that's a distinct variable scoped to the block — it never
// filters the outer q used by .All(ctx), which must still be flagged.
func bad3(ctx context.Context, cond bool) {
	q := newQuery()
	if cond {
		q := q.Where("status = 1")
		_ = q
	}
	_, _ = q.All(ctx) // want `ent query \.All\(ctx\) with no \.Where`
}

// notFlagged: .All is called on a type that isn't a *Query — the check only
// applies to ent-shaped query builders.
type FakeList struct{}

func (l *FakeList) All(ctx context.Context) ([]int, error) { return nil, nil }

func notFlagged(ctx context.Context) {
	l := &FakeList{}
	_, _ = l.All(ctx)
}
