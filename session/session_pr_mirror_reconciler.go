package session

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

// MetricSessionPRMirrorDrift counts sessions whose stored legacy PR columns
// differed from the ActivePR projection (kind=corrected) and sessions left alone
// because their legacy number is unrepresented (kind=unrepresented_skipped).
const MetricSessionPRMirrorDrift = "ssq_session_pr_mirror_drift_total"

const (
	mirrorKindCorrected            = "corrected"
	mirrorKindUnrepresentedSkipped = "unrepresented_skipped"
)

// prMirrorReconcileInterval is the slow ticker period; drift is a safety net, not a hot path.
const prMirrorReconcileInterval = 30 * time.Minute

var prMirrorDrift = newPRMirrorDriftCounter()

type prMirrorDriftCounter struct {
	corrected, skipped atomic.Int64
	otel               metric.Int64Counter
}

func newPRMirrorDriftCounter() *prMirrorDriftCounter {
	c := &prMirrorDriftCounter{}
	if oc, err := telemetry.GetMeter().Int64Counter(MetricSessionPRMirrorDrift); err == nil {
		c.otel = oc
	}
	return c
}

func (c *prMirrorDriftCounter) add(kind string, n int64) {
	if n == 0 {
		return
	}
	if kind == mirrorKindCorrected {
		c.corrected.Add(n)
	} else {
		c.skipped.Add(n)
	}
	if c.otel != nil {
		c.otel.Add(context.Background(), n, metric.WithAttributes(attribute.String("kind", kind)))
	}
}

// PRMirrorDriftCorrected is ssq_session_pr_mirror_drift_total{kind="corrected"} in this process.
func PRMirrorDriftCorrected() int64 { return prMirrorDrift.corrected.Load() }

// PRMirrorDriftSkipped is ssq_session_pr_mirror_drift_total{kind="unrepresented_skipped"} in this process.
func PRMirrorDriftSkipped() int64 { return prMirrorDrift.skipped.Load() }

// prMirrorStore is what the reconciler needs from storage.
type prMirrorStore interface {
	prMirrorPersister
	ListPRMirrors(ctx context.Context) (map[string]PRMirror, error)
}

// PRMirrorReconciler asserts every session's legacy PR columns equal its
// ActivePR projection. It always runs (no scope condition), corrects drift
// through the instance actor, and imports nothing (the boot backfill does).
type PRMirrorReconciler struct {
	instances func() []*Instance
	store     prMirrorStore
	interval  time.Duration
	wg        sync.WaitGroup
	started   atomic.Bool
}

// NewPRMirrorReconciler builds a reconciler over the instances instances returns.
func NewPRMirrorReconciler(instances func() []*Instance, store prMirrorStore) *PRMirrorReconciler {
	return &PRMirrorReconciler{instances: instances, store: store, interval: prMirrorReconcileInterval}
}

// RunOnce reconciles every instance and returns how many were corrected and how
// many were skipped as unrepresented. A failed stored-column read degrades to
// comparing the in-memory fields only.
func (r *PRMirrorReconciler) RunOnce(ctx context.Context) (corrected, skipped int) {
	stored, err := r.store.ListPRMirrors(ctx)
	if err != nil {
		log.Warn("pr mirror reconciler: stored columns unreadable; comparing in-memory fields only", "err", err)
	}
	for _, inst := range r.instances() {
		if ctx.Err() != nil {
			break
		}
		var prior *PRMirror
		if m, ok := stored[inst.Snapshot().UUID]; ok && err == nil {
			prior = &m
		}
		switch inst.ReconcilePRMirror(prior, r.store) {
		case mirrorCorrected:
			corrected++
		case mirrorUnrepresentedSkipped:
			skipped++
		case mirrorInSync:
		}
	}
	prMirrorDrift.add(mirrorKindCorrected, int64(corrected))
	prMirrorDrift.add(mirrorKindUnrepresentedSkipped, int64(skipped))
	if corrected > 0 || skipped > 0 {
		log.Info("session pr mirror reconcile", "corrected", corrected, "unrepresented_skipped", skipped)
	}
	return corrected, skipped
}

// Start runs one pass at once and then one per interval until ctx ends. A second
// call is a no-op.
func (r *PRMirrorReconciler) Start(ctx context.Context) {
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
func (r *PRMirrorReconciler) Wait() { r.wg.Wait() }
