// Package dbsplit routes ent statements between a single-connection writer
// pool and a multi-connection read pool over the same WAL-mode SQLite file.
//
// Routing is by statement text and fails safe: only a plain SELECT goes to a
// reader; everything else (writes, RETURNING, WITH, PRAGMA, anything
// unrecognised) and every transaction goes to the writer queue. Readers are
// opened query_only, so a misrouted write errors rather than corrupts.
package dbsplit

import (
	"context"
	"database/sql"

	"entgo.io/ent/dialect"
)

// Driver sends reads to reader and everything else to writer.
type Driver struct {
	writer, reader dialect.Driver
}

// New returns a driver that owns both: Close closes writer and reader.
func New(writer, reader dialect.Driver) *Driver {
	return &Driver{writer: writer, reader: reader}
}

func (d *Driver) Dialect() string { return d.writer.Dialect() }

func (d *Driver) Exec(ctx context.Context, q string, args, v any) error {
	return d.writer.Exec(ctx, q, args, v)
}

func (d *Driver) Query(ctx context.Context, q string, args, v any) error {
	if IsReadOnly(q) {
		return d.reader.Query(ctx, q, args, v)
	}
	return d.writer.Query(ctx, q, args, v)
}

// Tx always uses the writer: a transaction may write, and reads inside it
// must see its own uncommitted rows.
func (d *Driver) Tx(ctx context.Context) (dialect.Tx, error) { return d.writer.Tx(ctx) }

// BeginTx is required: ent's generated Client.BeginTx type-asserts it on the driver.
func (d *Driver) BeginTx(ctx context.Context, opts *sql.TxOptions) (dialect.Tx, error) {
	if o, ok := d.writer.(interface {
		BeginTx(context.Context, *sql.TxOptions) (dialect.Tx, error)
	}); ok {
		return o.BeginTx(ctx, opts)
	}
	return d.writer.Tx(ctx)
}

func (d *Driver) Close() error {
	werr := d.writer.Close()
	if rerr := d.reader.Close(); werr == nil {
		return rerr
	}
	return werr
}

// IsReadOnly reports whether q is a plain SELECT. Leading whitespace and
// SQL comments are skipped; the keyword must be followed by a non-identifier
// byte so identifiers like "selection" never match.
func IsReadOnly(q string) bool {
	i := skipLeading(q)
	const kw = "select"
	if len(q)-i < len(kw) {
		return false
	}
	for j := 0; j < len(kw); j++ {
		if q[i+j]|0x20 != kw[j] {
			return false
		}
	}
	if i+len(kw) == len(q) {
		return true
	}
	c := q[i+len(kw)]
	return !(c == '_' || c >= '0' && c <= '9' || c|0x20 >= 'a' && c|0x20 <= 'z')
}

func skipLeading(q string) int {
	i := 0
	for i < len(q) {
		switch {
		case q[i] == ' ' || q[i] == '\t' || q[i] == '\n' || q[i] == '\r' || q[i] == '(':
			i++
		case i+1 < len(q) && q[i] == '-' && q[i+1] == '-':
			for i < len(q) && q[i] != '\n' {
				i++
			}
		case i+1 < len(q) && q[i] == '/' && q[i+1] == '*':
			end := indexFrom(q, "*/", i+2)
			if end < 0 {
				return len(q)
			}
			i = end + 2
		default:
			return i
		}
	}
	return i
}

func indexFrom(s, sub string, from int) int {
	for i := from; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
