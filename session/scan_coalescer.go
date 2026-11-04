package session

import "sync"

// scanCoalescer collapses a burst of do calls into at most one trailing run.
// A caller that arrives while a run is in flight waits for a run that starts
// after its arrival, so it still observes any change that preceded the call,
// but N concurrent callers cost 2 runs instead of N.
type scanCoalescer struct {
	mu      sync.Mutex
	running bool
	joined  []chan struct{} // callers waiting for the next run
}

func (c *scanCoalescer) do(run func()) {
	c.mu.Lock()
	if c.running {
		ch := make(chan struct{})
		c.joined = append(c.joined, ch)
		c.mu.Unlock()
		<-ch
		return
	}
	c.running = true
	c.mu.Unlock()

	var batch []chan struct{}
	finished := false
	defer func() {
		if finished {
			return
		}
		// run panicked: release waiters and the running flag.
		c.mu.Lock()
		c.running = false
		pending := append(batch, c.joined...)
		c.joined = nil
		c.mu.Unlock()
		for _, ch := range pending {
			close(ch)
		}
	}()
	for {
		run()
		for _, ch := range batch {
			close(ch)
		}
		batch = nil
		c.mu.Lock()
		batch, c.joined = c.joined, nil
		if len(batch) == 0 {
			c.running = false
			finished = true
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()
	}
}
