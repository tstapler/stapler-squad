package debounce

import (
	"sync"
	"time"
)

// Debouncer provides a utility for delaying operations until input has settled
type Debouncer struct {
	delay    time.Duration
	timer    *time.Timer
	mutex    sync.Mutex
	callback func()
}

// New creates a new debouncer with the specified delay
func New(delay time.Duration) *Debouncer {
	return &Debouncer{
		delay: delay,
		mutex: sync.Mutex{},
	}
}

// Trigger schedules the callback to be executed after the delay
// If Trigger is called again before the delay expires, the previous call is cancelled
func (d *Debouncer) Trigger(callback func()) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	
	// Cancel any existing timer
	if d.timer != nil {
		d.timer.Stop()
	}
	
	// Store the callback
	d.callback = callback
	
	// Start a new timer
	d.timer = time.AfterFunc(d.delay, func() {
		d.mutex.Lock()
		callback := d.callback
		d.mutex.Unlock()
		
		if callback != nil {
			callback()
		}
	})
}

// Cancel stops any pending operation
func (d *Debouncer) Cancel() {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
}

// Execute cancels any pending operation and executes the callback immediately
func (d *Debouncer) Execute(callback func()) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	
	// Cancel any existing timer
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	
	// Execute the callback directly
	if callback != nil {
		callback()
	}
}

// SetDelay changes the delay duration for future triggers
func (d *Debouncer) SetDelay(delay time.Duration) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	
	d.delay = delay
}

// IsActive returns true if there is a pending operation
func (d *Debouncer) IsActive() bool {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	
	return d.timer != nil
}