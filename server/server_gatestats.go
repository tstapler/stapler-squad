package server

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/services"
)

// startGateStatsWriter persists the delivery gate's hourly buckets once a
// minute and registers the bounded final flush. A config-dir failure leaves the
// writer unstarted, which makes enabling the gate refuse (Task 2.8g).
func (s *Server) startGateStatsWriter(ctx context.Context, deps *ServerDependencies, gate *deliverygate.Gate, configDir string, configErr error) {
	if configErr != nil {
		log.Warn("delivery gate stats writer not started: no config dir", "err", configErr)
		return
	}
	store := services.NewFileStatsStore(configDir)
	writer := services.NewStatsWriter(gate.Stats(), store, nil, nil, nil)
	ticker := time.NewTicker(time.Minute)
	writer.Start(ctx, ticker.C, &s.backgroundTasksWG)
	deps.SessionService.SetGateStatsFileStatus(store.Status)
	s.shutdownHooks = append(s.shutdownHooks, ticker.Stop)
	s.finalStatsFlush = writer.FlushFinal
	gate.WarnIfEnabledWithoutStats()
}
