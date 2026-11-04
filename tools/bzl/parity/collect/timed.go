package collect

import "errors"

// ErrNotQuiet means a timed sample was refused or tainted by host load.
var ErrNotQuiet = errors.New("NOT QUIET")

// Sample runs one timed measurement n times under the gate. With Require set it refuses to start a sample
// while the host is loud and returns ErrNotQuiet with the readings so far; the caller must not publish a number.
func Sample(g Gate, n int, one func() (float64, error)) ([]float64, []Reading, error) {
	var vals []float64
	var rs []Reading
	for i := 0; i < n; i++ {
		r := g.Begin()
		if g.Require && r.Pre > g.Max {
			rs = append(rs, g.End(r))
			return vals, rs, ErrNotQuiet
		}
		v, err := one()
		r = g.End(r)
		rs = append(rs, r)
		if err != nil {
			return vals, rs, err
		}
		vals = append(vals, v)
	}
	return vals, rs, nil
}

// SetTimed records a timing value only when every sample was quiet; otherwise the row is UNMEASURED with the
// NOT QUIET reason, and with --require-quiet=false the measured number is kept as Advisory only.
func (d *Doc) SetTimed(key string, v float64, rs []Reading, g Gate, source string) {
	quiet := AllQuiet(rs)
	m := Meta{Quiet: &quiet, Load: MaxLoad(rs), Source: source}
	if quiet {
		d.Set(key, v, m)
		return
	}
	if !g.Require {
		m.Advisory = &v
	}
	d.Unmeasured(key, NotQuietReason(rs, g.Max), m)
}
