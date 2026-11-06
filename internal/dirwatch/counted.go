package dirwatch

import (
	"context"
	"path/filepath"
	"sync"

	"github.com/tstapler/stapler-squad/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// counted tracks the distinct paths added to inner so the total is observable
// and a runaway watcher is noticed. A repeated Add of the same path is a no-op
// here, which also dedupes across callers sharing one Watcher.
type counted struct {
	Watcher
	name   string
	warnAt int
	warn   func(name string, count, limit int) // seam: tests observe crossings

	mu     sync.Mutex
	paths  map[string]struct{}
	warned bool
}

func instrument(inner Watcher, cfg config) *counted {
	c := &counted{
		Watcher: inner, name: cfg.name, warnAt: cfg.warnAt, paths: make(map[string]struct{}),
		warn: func(name string, count, limit int) {
			log.Warn("dirwatch: watch count above threshold; something is watching more than it uses",
				"watcher", name, "watches", count, "threshold", limit)
		},
	}
	registerCounted(c)
	return c
}

func (c *counted) Add(name string) error {
	key := filepath.Clean(name)
	c.mu.Lock()
	_, dup := c.paths[key]
	c.mu.Unlock()
	if dup {
		return nil
	}
	if err := c.Watcher.Add(name); err != nil {
		return err
	}
	c.mu.Lock()
	c.paths[key] = struct{}{}
	n, crossed := len(c.paths), false
	if n > c.warnAt && !c.warned {
		c.warned, crossed = true, true
	}
	c.mu.Unlock()
	if crossed {
		c.warn(c.name, n, c.warnAt)
	}
	return nil
}

func (c *counted) Remove(name string) error {
	key := filepath.Clean(name)
	err := c.Watcher.Remove(name)
	c.mu.Lock()
	delete(c.paths, key)
	if len(c.paths) <= c.warnAt {
		c.warned = false
	}
	c.mu.Unlock()
	return err
}

func (c *counted) Close() error {
	unregisterCounted(c)
	return c.Watcher.Close()
}

func (c *counted) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.paths)
}

//nolint:gochecknoglobals // process-wide registry feeding the single fs.watches gauge
var (
	liveMu   sync.Mutex
	live     = map[*counted]struct{}{}
	gaugeOne sync.Once
)

func registerCounted(c *counted) {
	liveMu.Lock()
	live[c] = struct{}{}
	liveMu.Unlock()
	gaugeOne.Do(registerGauge)
}

func unregisterCounted(c *counted) {
	liveMu.Lock()
	delete(live, c)
	liveMu.Unlock()
}

func registerGauge() {
	m := otel.Meter("stapler-squad/dirwatch")
	g, err := m.Int64ObservableGauge("fs.watches",
		metric.WithDescription("Distinct paths currently watched, per watcher. Costs no fds under FSEvents; under kqueue (fallback or STAPLER_SQUAD_DIRWATCH=fsnotify) each costs 1 fd plus 1 per file in the dir"))
	if err != nil {
		log.Warn("dirwatch: register fs.watches gauge", "err", err)
		return
	}
	_, err = m.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		liveMu.Lock()
		defer liveMu.Unlock()
		byName := map[string]int{}
		for c := range live {
			byName[c.name] += c.count()
		}
		for name, n := range byName {
			o.ObserveInt64(g, int64(n), metric.WithAttributes(attribute.String("watcher", name)))
		}
		return nil
	}, g)
	if err != nil {
		log.Warn("dirwatch: register fs.watches callback", "err", err)
	}
}
