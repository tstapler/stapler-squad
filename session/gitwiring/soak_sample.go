package gitwiring

import (
	"context"
	"sort"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// counterPoint is one labelled counter value.
type counterPoint struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
	Value  int64             `json:"value"`
}

// histPoint is one labelled histogram. Buckets has len(Bounds)+1 entries (the last is overflow).
type histPoint struct {
	Name    string            `json:"name"`
	Labels  map[string]string `json:"labels,omitempty"`
	Bounds  []float64         `json:"bounds"`
	Buckets []uint64          `json:"buckets"`
	Count   uint64            `json:"count"`
}

// soakSample is a cumulative reading of the git backend instruments.
type soakSample struct {
	Counters   []counterPoint `json:"counters,omitempty"`
	Histograms []histPoint    `json:"histograms,omitempty"`
}

func pointKey(name string, labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(name)
	for _, k := range keys {
		b.WriteString("|" + k + "=" + labels[k])
	}
	return b.String()
}

func labelMap(set attribute.Set) map[string]string {
	out := make(map[string]string, set.Len())
	for iter := set.Iter(); iter.Next(); {
		kv := iter.Attribute()
		out[string(kv.Key)] = kv.Value.AsString()
	}
	return out
}

// readLive returns what this process has counted since start.
func readLive(ctx context.Context) soakSample {
	soakMeter() // ensure the reader is registered before collecting
	var rm metricdata.ResourceMetrics
	if err := soakReader.Collect(ctx, &rm); err != nil {
		return soakSample{}
	}
	var out soakSample
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch d := m.Data.(type) {
			case metricdata.Sum[int64]:
				for _, dp := range d.DataPoints {
					out.Counters = append(out.Counters, counterPoint{Name: m.Name, Labels: labelMap(dp.Attributes), Value: dp.Value})
				}
			case metricdata.Histogram[float64]:
				for _, dp := range d.DataPoints {
					out.Histograms = append(out.Histograms, histPoint{
						Name: m.Name, Labels: labelMap(dp.Attributes),
						Bounds: dp.Bounds, Buckets: dp.BucketCounts, Count: dp.Count,
					})
				}
			}
		}
	}
	return out
}

// mergeSamples adds b onto a (a persisted ledger plus this process's live counts). Histograms
// whose bucket layout differs are not summed; the newer one (b) wins.
func mergeSamples(a, b soakSample) soakSample {
	counters := map[string]*counterPoint{}
	var order []string
	add := func(p counterPoint) {
		k := pointKey(p.Name, p.Labels)
		if cur, ok := counters[k]; ok {
			cur.Value += p.Value
			return
		}
		cp := p
		counters[k] = &cp
		order = append(order, k)
	}
	for _, p := range a.Counters {
		add(p)
	}
	for _, p := range b.Counters {
		add(p)
	}
	var out soakSample
	sort.Strings(order)
	for _, k := range order {
		out.Counters = append(out.Counters, *counters[k])
	}

	hists := map[string]*histPoint{}
	var horder []string
	addH := func(p histPoint) {
		k := pointKey(p.Name, p.Labels)
		cur, ok := hists[k]
		if !ok || len(cur.Buckets) != len(p.Buckets) {
			cp := p
			cp.Buckets = append([]uint64(nil), p.Buckets...)
			hists[k] = &cp
			if !ok {
				horder = append(horder, k)
			}
			return
		}
		for i := range p.Buckets {
			cur.Buckets[i] += p.Buckets[i]
		}
		cur.Count += p.Count
	}
	for _, p := range a.Histograms {
		addH(p)
	}
	for _, p := range b.Histograms {
		addH(p)
	}
	sort.Strings(horder)
	for _, k := range horder {
		out.Histograms = append(out.Histograms, *hists[k])
	}
	return out
}

// p50 estimates the median of h by linear interpolation inside the bucket that crosses 50%.
// ok is false for an empty histogram. A median in the overflow bucket reports the last bound.
func (h histPoint) p50() (ms float64, ok bool) {
	if h.Count == 0 || len(h.Buckets) == 0 {
		return 0, false
	}
	half := float64(h.Count) / 2
	var cum float64
	for i, n := range h.Buckets {
		next := cum + float64(n)
		if next >= half && n > 0 {
			if i >= len(h.Bounds) {
				return h.Bounds[len(h.Bounds)-1], true
			}
			lower := 0.0
			if i > 0 {
				lower = h.Bounds[i-1]
			}
			return lower + (h.Bounds[i]-lower)*(half-cum)/float64(n), true
		}
		cum = next
	}
	return 0, false
}
