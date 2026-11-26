// Package prmirror asserts every session's legacy PR columns equal its ActivePR projection
// and counts the drift it corrects (ssq_session_pr_mirror_drift_total).
package prmirror

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/telemetry"
)

// MetricSessionPRMirrorDrift counts sessions whose stored legacy PR columns
// differed from the ActivePR projection (kind=corrected) and sessions left alone
// because their legacy number is unrepresented (kind=unrepresented_skipped).
const MetricSessionPRMirrorDrift = "ssq_session_pr_mirror_drift_total"

const (
	kindCorrected            = "corrected"
	kindUnrepresentedSkipped = "unrepresented_skipped"
)

// prMirrorReconcileInterval is the slow ticker period; drift is a safety net, not a hot path.
const prMirrorReconcileInterval = 30 * time.Minute

var prMirrorDrift = newDriftCounter()

type driftCounter struct {
	corrected, skipped atomic.Int64
	otel               metric.Int64Counter
}

func newDriftCounter() *driftCounter {
	c := &driftCounter{}
	if oc, err := telemetry.GetMeter().Int64Counter(MetricSessionPRMirrorDrift); err == nil {
		c.otel = oc
	}
	return c
}

func (c *driftCounter) add(kind string, n int64) {
	if n == 0 {
		return
	}
	if kind == kindCorrected {
		c.corrected.Add(n)
	} else {
		c.skipped.Add(n)
	}
	if c.otel != nil {
		c.otel.Add(context.Background(), n, metric.WithAttributes(attribute.String("kind", kind)))
	}
}

// DriftCorrected is ssq_session_pr_mirror_drift_total{kind="corrected"} in this process.
func DriftCorrected() int64 { return prMirrorDrift.corrected.Load() }

// DriftSkipped is ssq_session_pr_mirror_drift_total{kind="unrepresented_skipped"} in this process.
func DriftSkipped() int64 { return prMirrorDrift.skipped.Load() }

// prMirrorStore is what the reconciler needs from storage.
type prMirrorStore interface {
	instance.PrMirrorPersister
	ListPRMirrors(ctx context.Context) (map[string]instance.PRMirror, error)
}

// Reconciler asserts every session's legacy PR columns equal its
// ActivePR projection. It always runs (no scope condition), corrects drift
// through the instance actor, and imports nothing (the boot backfill does).
type Reconciler struct {
	instances func() []*instance.Instance
	store     prMirrorStore
	interval  time.Duration
	wg        sync.WaitGroup
	started   atomic.Bool
}

// NewReconciler builds a reconciler over the instances instances returns.
func NewReconciler(instances func() []*instance.Instance, store prMirrorStore) *Reconciler {
	return &Reconciler{instances: instances, store: store, interval: prMirrorReconcileInterval}
}

// RunOnce reconciles every instance and returns how many were corrected and how
// many were skipped as unrepresented. A failed stored-column read degrades to
// comparing the in-memory fields only.
func (r *Reconciler) RunOnce(ctx context.Context) (corrected, skipped int) {
	stored, err := r.store.ListPRMirrors(ctx)
	if err != nil {
		log.Warn("pr mirror reconciler: stored columns unreadable; comparing in-memory fields only", "err", err)
	}
	for _, inst := range r.instances() {
		if ctx.Err() != nil {
			break
		}
		var prior *instance.PRMirror
		if m, ok := stored[inst.Snapshot().UUID]; ok && err == nil {
			prior = &m
		}
		switch inst.ReconcilePRMirror(prior, r.store) {
		case instance.MirrorCorrected:
			corrected++
		case instance.MirrorUnrepresentedSkipped:
			skipped++
		case instance.MirrorInSync:
		}
	}
	prMirrorDrift.add(kindCorrected, int64(corrected))
	prMirrorDrift.add(kindUnrepresentedSkipped, int64(skipped))
	if corrected > 0 || skipped > 0 {
		log.Info("session pr mirror reconcile", "corrected", corrected, "unrepresented_skipped", skipped)
	}
	return corrected, skipped
}

// Start runs one pass at once and then one per interval until ctx ends. A second
// call is a no-op.
func (r *Reconciler) Start(ctx context.Context) {
	if !r.started.CompareAndSwap(false, true) {
		return
	}
	r.wg.Go(func() {
		r.RunOnce(ctx)
		t := time.NewTicker(r.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.RunOnce(ctx)
			}
		}
	})
}

// Wait blocks until the loop has exited (after its context is cancelled).
func (r *Reconciler) Wait() { r.wg.Wait() }
