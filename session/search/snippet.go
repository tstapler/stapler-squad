package search

import (
	"strings"
	"time"
	"unicode"
)

// Snippet represents a highlighted text snippet showing where a search term appears.
type Snippet struct {
	// Text is the snippet text with surrounding context
	Text string
	// HighlightRanges contains all highlight positions in the snippet
	HighlightRanges []HighlightRange
	// MessageRole is the role of the message sender (user, assistant, system)
	MessageRole string
	// MessageTime is when the message was created
	MessageTime time.Time
}

// HighlightRange represents a range to highlight in the snippet text.
type HighlightRange struct {
	Start int
	End   int
}

// SnippetGenerator generates contextual snippets from search results.
type SnippetGenerator struct {
	// contextWords is the number of words to include before and after a match
	contextWords int
	// maxSnippets is the maximum number of snippets to generate per message
	maxSnippets int
	// maxSnippetLength is the maximum length of a snippet in characters
	maxSnippetLength int
	// tokenizer is used to normalize query terms
	tokenizer *Tokenizer
}

// NewSnippetGenerator creates a new snippet generator with default settings.
func NewSnippetGenerator() *SnippetGenerator {
	return &SnippetGenerator{
		contextWords:     30,
		maxSnippets:      3,
		maxSnippetLength: 300,
		tokenizer:        NewTokenizer(),
	}
}

// NewSnippetGeneratorWithOptions creates a snippet generator with custom settings.
func NewSnippetGeneratorWithOptions(contextWords, maxSnippets, maxSnippetLength int) *SnippetGenerator {
	return &SnippetGenerator{
		contextWords:     contextWords,
		maxSnippets:      maxSnippets,
		maxSnippetLength: maxSnippetLength,
		tokenizer:        NewTokenizer(),
	}
}

// Generate creates snippets from a message containing the query terms.
// query should be the original query terms (will be tokenized for matching).
func (g *SnippetGenerator) Generate(message string, query string, role string, timestamp time.Time) []Snippet {
	if message == "" || query == "" {
		return nil
	}

	// Tokenize query to get normalized terms
	queryTerms := g.tokenizer.Tokenize(query)
	if len(queryTerms) == 0 {
		return nil
	}

	// Find all match positions in the original message
	matches := g.findMatchPositions(message, queryTerms)
	if len(matches) == 0 {
		return nil
	}

	// Merge overlapping match ranges
	mergedMatches := g.mergeOverlappingMatches(matches)

	// Generate snippets for each match (up to maxSnippets)
	snippets := make([]Snippet, 0, g.maxSnippets)
	usedRanges := make([]struct{ start, end int }, 0)

	for _, match := range mergedMatches {
		if len(snippets) >= g.maxSnippets {
			break
		}

		// Skip if this match overlaps with an already-used snippet range
		if g.overlapsWithExisting(match.charStart, match.charEnd, usedRanges) {
			continue
		}

		snippet := g.extractSnippetAtPosition(message, match.charStart, match.charEnd, queryTerms)
		snippet.MessageRole = role
		snippet.MessageTime = timestamp

		snippets = append(snippets, snippet)
		usedRanges = append(usedRanges, struct{ start, end int }{
			start: match.charStart - g.contextWords*5, // approximate character range
			end:   match.charEnd + g.contextWords*5,
		})
	}

	return snippets
}

// matchPosition represents a match in the message.
type matchPosition struct {
	charStart int // start character position in original message
	charEnd   int // end character position in original message
	term      string
}

// findMatchPositions finds all positions where query terms appear in the message.
func (g *SnippetGenerator) findMatchPositions(message string, queryTerms []string) []matchPosition {
	matches := make([]matchPosition, 0)
	messageLower := strings.ToLower(message)

	// Build a set of stemmed query terms for matching
	stemmedTerms := make(map[string]string) // stemmed -> original
	for _, term := range queryTerms {
		stemmedTerms[term] = term
	}

	// Scan through the message word by word
	wordStart := -1
	for i := 0; i <= len(message); i++ {
		isWordChar := i < len(message) && (unicode.IsLetter(rune(message[i])) || unicode.IsDigit(rune(message[i])))

		if isWordChar && wordStart == -1 {
			wordStart = i
		} else if !isWordChar && wordStart != -1 {
			// End of word
			wordLower := messageLower[wordStart:i]

			// Check if this word matches any query term (with stemming)
			wordStemmed := g.tokenizer.StemWord(wordLower)

			for stemmed := range stemmedTerms {
				// Match if stemmed forms match or if there's a substring match
				if wordStemmed == stemmed || strings.Contains(wordLower, stemmed) || strings.Contains(stemmed, wordLower) {
					matches = append(matches, matchPosition{
						charStart: wordStart,
						charEnd:   i,
						term:      stemmed,
					})
					break
				}
			}

			wordStart = -1
		}
	}

	return matches
}

// mergeOverlappingMatches merges matches that are close together.
func (g *SnippetGenerator) mergeOverlappingMatches(matches []matchPosition) []matchPosition {
	if len(matches) == 0 {
		return matches
	}

	// Sort by start position (already sorted from scanning)
	merged := make([]matchPosition, 0, len(matches))
	merged = append(merged, matches[0])

	for i := 1; i < len(matches); i++ {
		last := &merged[len(merged)-1]
		curr := matches[i]

		// Merge if within 50 characters (about 10 words)
		if curr.charStart <= last.charEnd+50 {
			if curr.charEnd > last.charEnd {
				last.charEnd = curr.charEnd
			}
		} else {
			merged = append(merged, curr)
		}
	}

	return merged
}

// overlapsWithExisting checks if a range overlaps with any existing ranges.
func (g *SnippetGenerator) overlapsWithExisting(start, end int, existing []struct{ start, end int }) bool {
	for _, r := range existing {
		if start <= r.end && end >= r.start {
			return true
		}
	}
	return false
}

// extractSnippetAtPosition extracts a snippet around the given character position.
func (g *SnippetGenerator) extractSnippetAtPosition(message string, matchStart, matchEnd int, queryTerms []string) Snippet {
	// Find word boundaries for context window
	snippetStart := g.findSnippetStart(message, matchStart)
	snippetEnd := g.findSnippetEnd(message, matchEnd)

	// Ensure valid range
	if snippetStart > snippetEnd {
		snippetStart = matchStart
	}
	if snippetEnd > len(message) {
		snippetEnd = len(message)
	}

	// Enforce max snippet length from start
	if snippetEnd-snippetStart > g.maxSnippetLength {
		// Find a word boundary near maxSnippetLength
		newEnd := snippetStart + g.maxSnippetLength
		if newEnd > len(message) {
			newEnd = len(message)
		}
		for newEnd > snippetStart && newEnd < len(message) && !unicode.IsSpace(rune(message[newEnd])) {
			newEnd--
		}
		if newEnd > snippetStart {
			snippetEnd = newEnd
		}
	}

	// Extract snippet text
	snippetText := message[snippetStart:snippetEnd]

	// Add ellipsis if truncated
	if snippetStart > 0 {
		snippetText = "..." + snippetText
	}
	if snippetEnd < len(message) {
		snippetText = snippetText + "..."
	}

	// Find all highlight positions within the snippet
	highlights := g.findHighlightsInSnippet(snippetText, queryTerms)

	return Snippet{
		Text:            snippetText,
		HighlightRanges: highlights,
	}
}

// findSnippetStart finds the start position for a snippet, respecting word boundaries.
func (g *SnippetGenerator) findSnippetStart(message string, matchStart int) int {
	// Count words backwards from match
	wordCount := 0
	pos := matchStart

	// First, move to the start of the match word
	for pos > 0 && !unicode.IsSpace(rune(message[pos-1])) {
		pos--
	}

	// Then count contextWords words backwards
	for pos > 0 && wordCount < g.contextWords {
		// Skip whitespace
		for pos > 0 && unicode.IsSpace(rune(message[pos-1])) {
			pos--
		}
		// Skip word
		for pos > 0 && !unicode.IsSpace(rune(message[pos-1])) {
			pos--
		}
		wordCount++
	}

	// Skip any leading whitespace
	for pos < len(message) && unicode.IsSpace(rune(message[pos])) {
		pos++
	}

	return pos
}

// findSnippetEnd finds the end position for a snippet, respecting word boundaries.
func (g *SnippetGenerator) findSnippetEnd(message string, matchEnd int) int {
	// Count words forward from match
	wordCount := 0
	pos := matchEnd

	// Ensure we start within bounds
	if pos >= len(message) {
		return len(message)
	}

	// First, move to the end of the match word
	for pos < len(message) && !unicode.IsSpace(rune(message[pos])) {
		pos++
	}

	// Then count contextWords words forward
	for pos < len(message) && wordCount < g.contextWords {
		// Skip whitespace
		for pos < len(message) && unicode.IsSpace(rune(message[pos])) {
			pos++
		}
		// Skip word
		for pos < len(message) && !unicode.IsSpace(rune(message[pos])) {
			pos++
		}
		wordCount++
	}

	return pos
}

// findHighlightsInSnippet finds all query term positions in the snippet for highlighting.
func (g *SnippetGenerator) findHighlightsInSnippet(snippet string, queryTerms []string) []HighlightRange {
	highlights := make([]HighlightRange, 0)
	snippetLower := strings.ToLower(snippet)

	// Scan through snippet word by word
	wordStart := -1
	for i := 0; i <= len(snippet); i++ {
		isWordChar := i < len(snippet) && (unicode.IsLetter(rune(snippet[i])) || unicode.IsDigit(rune(snippet[i])))

		if isWordChar && wordStart == -1 {
			wordStart = i
		} else if !isWordChar && wordStart != -1 {
			// End of word
			wordLower := snippetLower[wordStart:i]
			wordStemmed := g.tokenizer.StemWord(wordLower)

			for _, term := range queryTerms {
				if wordStemmed == term || strings.Contains(wordLower, term) || strings.Contains(term, wordLower) {
					highlights = append(highlights, HighlightRange{
						Start: wordStart,
						End:   i,
					})
					break
				}
			}

			wordStart = -1
		}
	}

	return highlights
}

// GenerateFromSearchResult generates snippets for a search result.
// This is a convenience method that uses the document content and query tokens.
func (g *SnippetGenerator) GenerateFromSearchResult(doc *Document, queryTokens []string) []Snippet {
	if doc == nil {
		return nil
	}

	query := strings.Join(queryTokens, " ")
	return g.Generate(doc.Content, query, doc.MessageRole, doc.Timestamp)
}
