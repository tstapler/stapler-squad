package collect

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
)

// Status values of a measured row.
const (
	Measured   = "MEASURED"
	Unmeasured = "UNMEASURED"
)

// Meta explains one row in the "_meta" object (the improvement evaluator ignores non-numeric keys).
type Meta struct {
	Status string  `json:"status"`
	Reason string  `json:"reason,omitempty"`
	Quiet  *bool   `json:"quiet,omitempty"`
	Load   float64 `json:"max_load,omitempty"`
	Source string  `json:"source,omitempty"`
	// Advisory is a number taken on a NOT QUIET host (only with --require-quiet=false); never a verdict.
	Advisory *float64 `json:"advisory_value,omitempty"`
}

// Doc accumulates rows produced by one collector run.
type Doc struct {
	vals map[string]float64
	meta map[string]Meta
}

// NewDoc returns an empty Doc.
func NewDoc() *Doc { return &Doc{vals: map[string]float64{}, meta: map[string]Meta{}} }

// Set records a measured value. NaN is refused and recorded as UNMEASURED.
func (d *Doc) Set(key string, v float64, m Meta) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		d.Unmeasured(key, "value not computable", m)
		return
	}
	m.Status = Measured
	d.vals[key], d.meta[key] = v, m
}

// Unmeasured records that key has no verdict number, and why.
func (d *Doc) Unmeasured(key, reason string, m Meta) {
	m.Status, m.Reason = Unmeasured, reason
	delete(d.vals, key)
	d.meta[key] = m
}

// Value returns a measured value.
func (d *Doc) Value(key string) (float64, bool) { v, ok := d.vals[key]; return v, ok }

// Meta returns the row metadata.
func (d *Doc) Meta(key string) Meta { return d.meta[key] }

// Keys lists every recorded row, sorted.
func (d *Doc) Keys() []string {
	out := make([]string, 0, len(d.meta))
	for k := range d.meta {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Merge writes d into the JSON file at path, keeping rows other collectors wrote. An UNMEASURED row removes any
// stale numeric value of the same key, so an old number can never outlive a failed re-measurement.
func Merge(path string, d *Doc) error {
	cur := map[string]any{}
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		if err := json.Unmarshal(b, &cur); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	meta, _ := cur["_meta"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
	}
	for k, m := range d.meta {
		meta[k] = m
		if v, ok := d.vals[k]; ok {
			cur[k] = v
		} else {
			delete(cur, k)
		}
	}
	cur["_meta"] = meta
	b, err := json.MarshalIndent(cur, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// Numbers loads the numeric rows of a measurements file, dropping "_meta" and any non-numeric value.
func Numbers(path string) (map[string]float64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	all := map[string]any{}
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, err
	}
	out := map[string]float64{}
	for k, v := range all {
		if f, ok := v.(float64); ok {
			out[k] = f
		}
	}
	return out, nil
}
