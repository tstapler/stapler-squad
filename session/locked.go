package session

import "sync"

// Locked bundles a value T with a RWMutex, enforcing lock discipline by only
// exposing the value through Read/Write callbacks.
//
// This is the Go equivalent of Rust's RwLock<T>: the data and its lock are
// inseparable, making it structurally impossible to access the value without
// correct lock discipline. Instead of a mutex sitting next to a field (which
// the compiler cannot enforce is held on access), callers receive or mutate
// the value only through Read or Write.
//
//	var listeners Locked[[]StatusChangeListener]
//
//	// Add a listener — write lock taken automatically
//	listeners.Write(func(ls *[]StatusChangeListener) {
//	    *ls = append(*ls, fn)
//	})
//
//	// Read all listeners — read lock taken automatically
//	var snapshot []StatusChangeListener
//	listeners.Read(func(ls []StatusChangeListener) {
//	    snapshot = append(snapshot, ls...)
//	})
type Locked[T any] struct {
	mu  sync.RWMutex
	val T // +checklocks:mu
}

// Read calls fn with a read-only copy of the value, holding the read lock for
// the duration. Multiple goroutines may call Read concurrently.
// Errors from fn should be captured via closure variables.
func (l *Locked[T]) Read(fn func(T)) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	fn(l.val)
}

// Write calls fn with a pointer to the value, holding the exclusive write lock
// for the duration. fn may mutate the value freely.
// Errors from fn should be captured via closure variables.
func (l *Locked[T]) Write(fn func(*T)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fn(&l.val)
}
