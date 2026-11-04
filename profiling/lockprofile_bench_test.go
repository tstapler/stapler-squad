package profiling

import (
	"runtime"
	"sync"
	"testing"
)

// Benchmarks behind the "Sampling cost" table in docs/observability/README.md.
// Each applies a sampling setting and resets it afterwards.

func benchContendedMutex(b *testing.B, fraction int) {
	runtime.SetMutexProfileFraction(fraction)
	defer runtime.SetMutexProfileFraction(0)
	var mu sync.Mutex
	n := 0
	b.SetParallelism(4)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			n++
			mu.Unlock()
		}
	})
	_ = n
}

func benchChannelPingPong(b *testing.B, rateNs int) {
	runtime.SetBlockProfileRate(rateNs)
	defer runtime.SetBlockProfileRate(0)
	in, out := make(chan int), make(chan int)
	go func() {
		for v := range in {
			out <- v
		}
	}()
	for i := 0; i < b.N; i++ {
		in <- i
		<-out
	}
	close(in)
}

func BenchmarkLockProfileMutexOff(b *testing.B)        { benchContendedMutex(b, 0) }
func BenchmarkLockProfileMutexFraction10(b *testing.B) { benchContendedMutex(b, DefaultMutexFraction) }
func BenchmarkLockProfileMutexFraction1(b *testing.B)  { benchContendedMutex(b, 1) }
func BenchmarkLockProfileBlockOff(b *testing.B)        { benchChannelPingPong(b, 0) }
func BenchmarkLockProfileBlockRate1000(b *testing.B)   { benchChannelPingPong(b, DefaultBlockRateNs) }
func BenchmarkLockProfileBlockRate1(b *testing.B)      { benchChannelPingPong(b, 1) }
