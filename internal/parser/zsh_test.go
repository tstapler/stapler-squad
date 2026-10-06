package parser

import (
	"context"
	"strings"
	"testing"
)

func TestZshParser(t *testing.T) {
	input := `: 1600000000:0;echo "hello"` + "\n" + `: 1600000005:0;ls -la` + "\n"

	p := NewZshParser()
	events, errs := p.Parse(context.Background(), strings.NewReader(input))

	var parsedEvents []string

	for e := range events {
		parsedEvents = append(parsedEvents, e.Command)
		if e.ProgramSource != "zsh" {
			t.Errorf("expected source to be zsh, got %s", e.ProgramSource)
		}
	}

	if err := <-errs; err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(parsedEvents) != 2 {
		t.Fatalf("expected 2 events, got %d", len(parsedEvents))
	}

	if parsedEvents[0] != `echo "hello"` {
		t.Errorf("expected echo \"hello\", got %s", parsedEvents[0])
	}
	if parsedEvents[1] != `ls -la` {
		t.Errorf("expected ls -la, got %s", parsedEvents[1])
	}
}
