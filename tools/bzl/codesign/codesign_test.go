package codesign

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeCodesign(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "codesign")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDesignated(t *testing.T) {
	bin := fakeCodesign(t, `echo "Executable=$3"; echo 'designated => identifier "com.stapler-squad" and anchor H"abc"'`)
	got, err := Designated(context.Background(), bin, "/x/stapler-squad")
	if err != nil {
		t.Fatal(err)
	}
	if want := `designated => identifier "com.stapler-squad" and anchor H"abc"`; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDesignatedErrors(t *testing.T) {
	if _, err := Designated(context.Background(), fakeCodesign(t, `echo nothing here`), "/x"); err == nil {
		t.Fatal("want error when no designated line is printed")
	}
	if _, err := Designated(context.Background(), fakeCodesign(t, `echo bad >&2; exit 1`), "/x"); err == nil || !strings.Contains(err.Error(), "bad") {
		t.Fatalf("want codesign failure with its output, got %v", err)
	}
}

func TestRecorded(t *testing.T) {
	doc := "intro\n" + beginMark + "\n```\ndesignated => anchor H\"abc\"\n```\n" + endMark + "\ntail designated => nope\n"
	got, err := Recorded(doc)
	if err != nil || got != `designated => anchor H"abc"` {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestRecordedMissing(t *testing.T) {
	for name, doc := range map[string]string{
		"no markers":  "nothing",
		"no end":      beginMark + "\ndesignated => x",
		"empty block": beginMark + "\n(not yet recorded)\n" + endMark,
	} {
		if _, err := Recorded(doc); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}
