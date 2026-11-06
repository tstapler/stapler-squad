package server

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/server/services"
)

// startLeaseWedgeWatcher scans for wedged terminal write leases every 10s and
// raises the once-per-episode tray warning (Story 5.0, Task 5.0f).
func (s *Server) startLeaseWedgeWatcher(ctx context.Context, deps *ServerDependencies) {
	ticker := time.NewTicker(services.LeaseWedgeScanInterval)
	deps.SessionService.StartLeaseWedgeWatcher(ctx, ticker.C, &s.backgroundTasksWG)
	s.shutdownHooks = append(s.shutdownHooks, ticker.Stop)
}
