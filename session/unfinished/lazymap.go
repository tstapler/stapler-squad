package unfinished

import (
	"sync/atomic"

	"github.com/puzpuzpuz/xsync/v4"
)

// lazyMap is a typed concurrent map whose zero value is usable (GoGitVCSReader
// is built as &GoGitVCSReader{}). Struct keys stay unboxed, so a Load hit
// allocates nothing, which sync.Map's `any` keys cannot promise. The map is
// created lazily by compare-and-swap; no lock is taken.
type lazyMap[K comparable, V any] struct {
	m atomic.Pointer[xsync.Map[K, V]]
}

func (l *lazyMap[K, V]) get() *xsync.Map[K, V] {
	if m := l.m.Load(); m != nil {
		return m
	}
	l.m.CompareAndSwap(nil, xsync.NewMap[K, V]())
	return l.m.Load()
}

func (l *lazyMap[K, V]) Load(k K) (V, bool) { return l.get().Load(k) }
func (l *lazyMap[K, V]) Store(k K, v V)     { l.get().Store(k, v) }
func (l *lazyMap[K, V]) Delete(k K)         { l.get().Delete(k) }

// Range visits every entry; stop by returning false.
func (l *lazyMap[K, V]) Range(f func(K, V) bool) { l.get().Range(f) }

// Len returns the number of entries.
func (l *lazyMap[K, V]) Len() int { return l.get().Size() }
