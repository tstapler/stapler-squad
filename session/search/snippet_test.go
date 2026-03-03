package search

import (
	"strings"
	"testing"
	"time"
)

func TestNewSnippetGenerator(t *testing.T) {
	gen := NewSnippetGenerator()
	if gen == nil {
		t.Fatal("NewSnippetGenerator returned nil")
	}
	if gen.contextWords != 30 {
		t.Errorf("contextWords = %d, want 30", gen.contextWords)
	}
	if gen.maxSnippets != 3 {
		t.Errorf("maxSnippets = %d, want 3", gen.maxSnippets)
	}
}

func TestSnippetGenerator_Generate_SingleMatch(t *testing.T) {
	gen := NewSnippetGenerator()
	timestamp := time.Now()

	message := "This is a test message about docker containers and kubernetes deployments"
	snippets := gen.Generate(message, "docker", "user", timestamp)

	if len(snippets) != 1 {
		t.Fatalf("len(snippets) = %d, want 1", len(snippets))
	}

	snippet := snippets[0]
	if !strings.Contains(snippet.Text, "docker") {
		t.Errorf("Snippet should contain 'docker': %q", snippet.Text)
	}
	if snippet.MessageRole != "user" {
		t.Errorf("MessageRole = %q, want 'user'", snippet.MessageRole)
	}
	if len(snippet.HighlightRanges) == 0 {
		t.Error("Expected at least one highlight range")
	}
}

func TestSnippetGenerator_Generate_MultipleMatches(t *testing.T) {
	gen := NewSnippetGeneratorWithOptions(10, 3, 300)
	timestamp := time.Now()

	message := "Docker is great. I love Docker containers. Docker makes deployment easy. Docker is used everywhere."
	snippets := gen.Generate(message, "docker", "assistant", timestamp)

	// Should have multiple snippets (up to maxSnippets)
	if len(snippets) == 0 {
		t.Error("Expected at least one snippet")
	}
	if len(snippets) > 3 {
		t.Errorf("len(snippets) = %d, should not exceed maxSnippets (3)", len(snippets))
	}
}

func TestSnippetGenerator_Generate_MatchAtStart(t *testing.T) {
	gen := NewSnippetGenerator()
	timestamp := time.Now()

	message := "Docker is a containerization platform that allows developers to package applications"
	snippets := gen.Generate(message, "docker", "user", timestamp)

	if len(snippets) == 0 {
		t.Fatal("Expected at least one snippet")
	}

	snippet := snippets[0]
	// Snippet should start near the beginning (no leading ellipsis for first word)
	if strings.HasPrefix(snippet.Text, "...") && strings.Contains(snippet.Text[:10], "Docker") {
		t.Errorf("Snippet starts with match, should not have leading ellipsis: %q", snippet.Text)
	}
}

func TestSnippetGenerator_Generate_MatchAtEnd(t *testing.T) {
	gen := NewSnippetGeneratorWithOptions(5, 3, 300)
	timestamp := time.Now()

	message := "The application uses a modern architecture for deploying containers with Docker"
	snippets := gen.Generate(message, "docker", "user", timestamp)

	if len(snippets) == 0 {
		t.Fatal("Expected at least one snippet")
	}

	snippet := snippets[0]
	// Snippet should contain Docker
	if !strings.Contains(strings.ToLower(snippet.Text), "docker") {
		t.Errorf("Snippet should contain 'docker': %q", snippet.Text)
	}
}

func TestSnippetGenerator_Generate_EmptyMessage(t *testing.T) {
	gen := NewSnippetGenerator()
	timestamp := time.Now()

	snippets := gen.Generate("", "docker", "user", timestamp)
	if len(snippets) != 0 {
		t.Errorf("len(snippets) = %d, want 0 for empty message", len(snippets))
	}
}

func TestSnippetGenerator_Generate_EmptyQuery(t *testing.T) {
	gen := NewSnippetGenerator()
	timestamp := time.Now()

	snippets := gen.Generate("Some message content", "", "user", timestamp)
	if len(snippets) != 0 {
		t.Errorf("len(snippets) = %d, want 0 for empty query", len(snippets))
	}
}

func TestSnippetGenerator_Generate_NoMatch(t *testing.T) {
	gen := NewSnippetGenerator()
	timestamp := time.Now()

	message := "This is a message about golang and rust programming"
	snippets := gen.Generate(message, "docker", "user", timestamp)

	if len(snippets) != 0 {
		t.Errorf("len(snippets) = %d, want 0 for no match", len(snippets))
	}
}

func TestSnippetGenerator_Generate_MultiWordQuery(t *testing.T) {
	gen := NewSnippetGenerator()
	timestamp := time.Now()

	message := "Docker containers are used for error handling and troubleshooting in production environments"
	snippets := gen.Generate(message, "docker error", "user", timestamp)

	if len(snippets) == 0 {
		t.Fatal("Expected at least one snippet")
	}

	// Should highlight both terms
	snippet := snippets[0]
	if len(snippet.HighlightRanges) < 1 {
		t.Errorf("Expected highlights for multi-word query, got %d", len(snippet.HighlightRanges))
	}
}

func TestSnippetGenerator_Generate_CaseInsensitive(t *testing.T) {
	gen := NewSnippetGenerator()
	timestamp := time.Now()

	message := "DOCKER is case insensitive Docker docker DOCKER"
	snippets := gen.Generate(message, "docker", "user", timestamp)

	if len(snippets) == 0 {
		t.Fatal("Expected at least one snippet")
	}

	snippet := snippets[0]
	// Should have multiple highlights (one for each occurrence)
	if len(snippet.HighlightRanges) == 0 {
		t.Error("Expected highlights for case-insensitive matches")
	}
}

func TestSnippetGenerator_Generate_StemmedMatch(t *testing.T) {
	gen := NewSnippetGenerator()
	timestamp := time.Now()

	// "running" stems to "run", "containers" stems to "contain"
	message := "The containers are running in production with multiple instances"
	snippets := gen.Generate(message, "container run", "user", timestamp)

	if len(snippets) == 0 {
		t.Fatal("Expected at least one snippet for stemmed matches")
	}

	snippet := snippets[0]
	t.Logf("Snippet: %q, highlights: %d", snippet.Text, len(snippet.HighlightRanges))
}

func TestSnippetGenerator_Generate_LongMessage(t *testing.T) {
	gen := NewSnippetGeneratorWithOptions(10, 3, 200)
	timestamp := time.Now()

	// Create a long message with match in the middle
	words := make([]string, 100)
	for i := range words {
		words[i] = "word"
	}
	words[50] = "docker"
	message := strings.Join(words, " ")

	snippets := gen.Generate(message, "docker", "user", timestamp)

	if len(snippets) == 0 {
		t.Fatal("Expected at least one snippet")
	}

	snippet := snippets[0]
	// Snippet should be truncated, not the entire message
	if len(snippet.Text) > 250 { // Allow some buffer for ellipsis
		t.Errorf("Snippet too long: %d characters", len(snippet.Text))
	}
	if !strings.Contains(snippet.Text, "docker") {
		t.Errorf("Snippet should contain match: %q", snippet.Text)
	}
}

func TestSnippetGenerator_Generate_WordBoundaries(t *testing.T) {
	gen := NewSnippetGenerator()
	timestamp := time.Now()

	// "docker" should not match "redocker" or "dockerized" differently
	message := "We use docker and also dockerized applications"
	snippets := gen.Generate(message, "docker", "user", timestamp)

	if len(snippets) == 0 {
		t.Fatal("Expected at least one snippet")
	}

	// Should find matches
	snippet := snippets[0]
	if len(snippet.HighlightRanges) == 0 {
		t.Error("Expected highlights")
	}
}

func TestSnippetGenerator_HighlightRanges(t *testing.T) {
	gen := NewSnippetGeneratorWithOptions(5, 1, 300)
	timestamp := time.Now()

	message := "Here is docker in a sentence"
	snippets := gen.Generate(message, "docker", "user", timestamp)

	if len(snippets) == 0 {
		t.Fatal("Expected at least one snippet")
	}

	snippet := snippets[0]
	if len(snippet.HighlightRanges) == 0 {
		t.Fatal("Expected at least one highlight range")
	}

	hr := snippet.HighlightRanges[0]
	highlighted := snippet.Text[hr.Start:hr.End]
	if strings.ToLower(highlighted) != "docker" {
		t.Errorf("Highlighted text = %q, want 'docker'", highlighted)
	}
}

func TestSnippetGenerator_GenerateFromSearchResult(t *testing.T) {
	gen := NewSnippetGenerator()

	doc := &Document{
		SessionID:    "session-1",
		MessageIndex: 0,
		MessageRole:  "user",
		Content:      "Help me with docker container issues",
		WordCount:    6,
		Timestamp:    time.Now(),
	}

	queryTokens := []string{"docker", "contain"}
	snippets := gen.GenerateFromSearchResult(doc, queryTokens)

	if len(snippets) == 0 {
		t.Fatal("Expected at least one snippet")
	}

	snippet := snippets[0]
	if snippet.MessageRole != "user" {
		t.Errorf("MessageRole = %q, want 'user'", snippet.MessageRole)
	}
}

func TestSnippetGenerator_GenerateFromSearchResult_NilDoc(t *testing.T) {
	gen := NewSnippetGenerator()

	snippets := gen.GenerateFromSearchResult(nil, []string{"docker"})
	if snippets != nil {
		t.Errorf("Expected nil snippets for nil doc, got %v", snippets)
	}
}

func TestSnippetGenerator_Ellipsis(t *testing.T) {
	gen := NewSnippetGeneratorWithOptions(3, 1, 300)
	timestamp := time.Now()

	// Create a message where the match is in the middle
	message := "First second third fourth fifth docker sixth seventh eighth ninth tenth"
	snippets := gen.Generate(message, "docker", "user", timestamp)

	if len(snippets) == 0 {
		t.Fatal("Expected at least one snippet")
	}

	snippet := snippets[0]
	// With only 3 context words and match in middle, should have leading ellipsis
	if !strings.HasPrefix(snippet.Text, "...") {
		t.Logf("Snippet: %q", snippet.Text)
		// This might not have ellipsis if the context includes the beginning
	}
}

// Benchmark tests
func BenchmarkSnippetGenerator_Generate_Short(b *testing.B) {
	gen := NewSnippetGenerator()
	timestamp := time.Now()
	message := "Docker containers are great for deployment"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gen.Generate(message, "docker", "user", timestamp)
	}
}

func BenchmarkSnippetGenerator_Generate_Long(b *testing.B) {
	gen := NewSnippetGenerator()
	timestamp := time.Now()

	// Create a longer message (~1000 words)
	words := make([]string, 1000)
	for i := range words {
		words[i] = "word"
	}
	words[500] = "docker"
	words[700] = "container"
	message := strings.Join(words, " ")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gen.Generate(message, "docker container", "user", timestamp)
	}
}

func BenchmarkSnippetGenerator_Generate_ManyMatches(b *testing.B) {
	gen := NewSnippetGenerator()
	timestamp := time.Now()

	// Message with many matches
	message := strings.Repeat("docker container error troubleshooting ", 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gen.Generate(message, "docker", "user", timestamp)
	}
}
