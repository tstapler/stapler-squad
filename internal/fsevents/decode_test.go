package fsevents

import (
	"testing"
	"unsafe"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func cstr(s string) *byte {
	b := append([]byte(s), 0)
	return &b[0]
}

func TestDecodeEvents(t *testing.T) {
	paths := []*byte{cstr("/a/b"), cstr("/c")}
	flags := []uint32{uint32(ItemCreated | ItemIsFile), uint32(HistoryDone)}
	ids := []uint64{7, 9}
	got := decodeEvents(2, unsafe.Pointer(&paths[0]), unsafe.Pointer(&flags[0]), unsafe.Pointer(&ids[0]))
	want := []Event{{"/a/b", ItemCreated | ItemIsFile, 7}, {"/c", HistoryDone, 9}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %+v want %+v", got, want)
	}
	if decodeEvents(0, nil, nil, nil) != nil {
		t.Fatal("expected nil for empty batch")
	}
}

func TestDispatchRescanNotSwallowed(t *testing.T) {
	for _, f := range []EventFlags{UserDropped, KernelDropped, MustScanSubDirs, EventIdsWrapped, RootChanged} {
		var rescans, batches int
		c := Config{
			OnEvents: func([]Event) { batches++ },
			OnRescan: func(Event) { rescans++ },
		}
		c.dispatch([]Event{{Path: "/x", Flags: ItemModified}, {Path: "/y", Flags: f}})
		if rescans != 1 || batches != 1 {
			t.Errorf("flag %#x: rescans=%d batches=%d", f, rescans, batches)
		}
	}
	c := Config{OnRescan: func(Event) { t.Error("HistoryDone must not trigger rescan") }}
	c.dispatch([]Event{{Flags: HistoryDone | ItemModified}})
}
