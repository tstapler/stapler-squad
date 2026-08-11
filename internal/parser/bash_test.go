package parser

import (
	"context"
	"strings"
	"testing"
)

func TestBashParser(t *testing.T) {
	input := "#1600000000\necho \"hello\"\n#1600000005\nls -la\n"
	
	p := NewBashParser()
	events, errs := p.Parse(context.Background(), strings.NewReader(input))
	
	var parsedEvents []string
	
	for e := range events {
		parsedEvents = append(parsedEvents, e.Command)
		if e.ProgramSource != "bash" {
			t.Errorf("expected source to be bash, got %s", e.ProgramSource)
		}
		if e.Timestamp.Unix() == 0 {
			t.Errorf("expected timestamp to be parsed, got zero time")
		}
	}
	
	if err := <-errs; err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	if len(parsedEvents) != 2 {
		t.Fatalf("expected 2 events, got %d", len(parsedEvents))
	}
	
	if parsedEvents[0] != "echo \"hello\"" {
		t.Errorf("expected echo \"hello\", got %s", parsedEvents[0])
	}
	if parsedEvents[1] != "ls -la" {
		t.Errorf("expected ls -la, got %s", parsedEvents[1])
	}
}
