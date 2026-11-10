package termlat

import (
	"testing"
	"time"
)

// Cost of one stage observation: window insert plus histogram record.
func BenchmarkRecord(b *testing.B) {
	r := New(nil)
	l := Labels{Backend: "tymux", Listener: "local"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Record(StageStreamSend, l, 3*time.Millisecond)
	}
}

func BenchmarkRecordParallel(b *testing.B) {
	r := New(nil)
	l := Labels{Backend: "tymux", Listener: "local"}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			r.Record(StageStreamSend, l, 3*time.Millisecond)
		}
	})
}
