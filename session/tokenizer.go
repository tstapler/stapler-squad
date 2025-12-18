package session

import (
	"strings"
	"unicode"
)

// Tokenizer handles text tokenization for full-text search.
// It provides lowercasing, stop word removal, and Porter stemming.
type Tokenizer struct {
	stopWords map[string]bool
}

// NewTokenizer creates a new Tokenizer with default English stop words.
func NewTokenizer() *Tokenizer {
	// Common English stop words that have low semantic value for search
	stopWords := map[string]bool{
		// Articles
		"a": true, "an": true, "the": true,
		// Conjunctions
		"and": true, "or": true, "but": true, "nor": true,
		// Prepositions
		"in": true, "on": true, "at": true, "to": true, "for": true,
		"of": true, "with": true, "by": true, "from": true, "as": true,
		// Pronouns
		"i": true, "me": true, "my": true, "we": true, "our": true,
		"you": true, "your": true, "he": true, "she": true, "it": true,
		"they": true, "them": true, "this": true, "that": true, "these": true,
		// Common verbs (forms of to be, to have, to do)
		"is": true, "are": true, "was": true, "were": true, "be": true,
		"been": true, "being": true, "am": true,
		"have": true, "has": true, "had": true, "having": true,
		"do": true, "does": true, "did": true, "doing": true,
		// Auxiliaries
		"will": true, "would": true, "could": true, "should": true,
		"can": true, "may": true, "might": true, "must": true,
		// Others
		"not": true, "no": true, "so": true, "if": true, "then": true,
		"than": true, "when": true, "what": true, "which": true, "who": true,
		"how": true, "all": true, "each": true, "both": true, "more": true,
		"most": true, "other": true, "some": true, "such": true, "only": true,
		"same": true, "just": true, "also": true, "very": true, "too": true,
	}

	return &Tokenizer{
		stopWords: stopWords,
	}
}

// Tokenize splits text into normalized tokens suitable for indexing.
// It performs: lowercase, word splitting, stop word removal, and stemming.
func (t *Tokenizer) Tokenize(text string) []string {
	if text == "" {
		return nil
	}

	// Lowercase for case-insensitive matching
	text = strings.ToLower(text)

	// Split on non-alphanumeric characters
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})

	// Filter stop words, short words, and apply stemming
	tokens := make([]string, 0, len(words))
	seen := make(map[string]bool) // Deduplicate within single text

	for _, word := range words {
		// Skip very short words (likely not meaningful)
		if len(word) < 2 {
			continue
		}

		// Skip stop words
		if t.stopWords[word] {
			continue
		}

		// Apply Porter stemming
		stemmed := porterStem(word)

		// Skip if we've already seen this stem in this text
		if seen[stemmed] {
			continue
		}
		seen[stemmed] = true

		tokens = append(tokens, stemmed)
	}

	return tokens
}

// TokenizeWithPositions returns tokens along with their character positions.
// This is useful for highlighting search results.
func (t *Tokenizer) TokenizeWithPositions(text string) []TokenPosition {
	if text == "" {
		return nil
	}

	lowerText := strings.ToLower(text)
	var positions []TokenPosition

	// Track word boundaries
	inWord := false
	wordStart := 0

	for i, r := range lowerText {
		isWordChar := unicode.IsLetter(r) || unicode.IsNumber(r)

		if isWordChar && !inWord {
			// Starting a new word
			inWord = true
			wordStart = i
		} else if !isWordChar && inWord {
			// Ending a word
			inWord = false
			word := lowerText[wordStart:i]

			if len(word) >= 2 && !t.stopWords[word] {
				stemmed := porterStem(word)
				positions = append(positions, TokenPosition{
					Token: stemmed,
					Start: wordStart,
					End:   i,
				})
			}
		}
	}

	// Handle last word if text doesn't end with separator
	if inWord {
		word := lowerText[wordStart:]
		if len(word) >= 2 && !t.stopWords[word] {
			stemmed := porterStem(word)
			positions = append(positions, TokenPosition{
				Token: stemmed,
				Start: wordStart,
				End:   len(lowerText),
			})
		}
	}

	return positions
}

// TokenPosition represents a token with its position in the original text.
type TokenPosition struct {
	Token string // The stemmed token
	Start int    // Start character position in original text
	End   int    // End character position in original text
}

// IsStopWord returns true if the word is a stop word.
func (t *Tokenizer) IsStopWord(word string) bool {
	return t.stopWords[strings.ToLower(word)]
}

// StemWord applies Porter stemming to a single word.
// The word should be lowercase.
func (t *Tokenizer) StemWord(word string) string {
	return porterStem(strings.ToLower(word))
}

// porterStem applies a simplified Porter stemming algorithm.
// This implements the core suffix stripping rules for English.
func porterStem(word string) string {
	if len(word) <= 2 {
		return word
	}

	// Step 1a: Remove plural forms
	word = step1a(word)

	// Step 1b: Remove -ed, -ing
	word = step1b(word)

	// Step 1c: Replace -y with -i
	word = step1c(word)

	// Step 2: Remove derivational suffixes
	word = step2(word)

	// Step 3: Remove derivational suffixes
	word = step3(word)

	// Step 4: Remove derivational suffixes
	word = step4(word)

	// Step 5: Clean up
	word = step5(word)

	return word
}

// Helper: count consonant-vowel sequences (measure)
func measure(word string) int {
	// Count VC sequences
	m := 0
	prevVowel := false

	for _, r := range word {
		isVowel := isVowelRune(r)
		if prevVowel && !isVowel {
			m++
		}
		prevVowel = isVowel
	}

	return m
}

func isVowelRune(r rune) bool {
	return r == 'a' || r == 'e' || r == 'i' || r == 'o' || r == 'u'
}

func isVowelAt(word string, i int) bool {
	if i < 0 || i >= len(word) {
		return false
	}
	r := rune(word[i])
	if r == 'y' {
		// y is a vowel if preceded by a consonant
		if i == 0 {
			return false
		}
		return !isVowelRune(rune(word[i-1]))
	}
	return isVowelRune(r)
}

func containsVowel(word string) bool {
	for i := range word {
		if isVowelAt(word, i) {
			return true
		}
	}
	return false
}

func endsWithDoubleConsonant(word string) bool {
	n := len(word)
	if n < 2 {
		return false
	}
	return word[n-1] == word[n-2] && !isVowelAt(word, n-1)
}

func endsCVC(word string) bool {
	n := len(word)
	if n < 3 {
		return false
	}
	// consonant-vowel-consonant where last consonant is not w, x, y
	last := word[n-1]
	if last == 'w' || last == 'x' || last == 'y' {
		return false
	}
	return !isVowelAt(word, n-1) && isVowelAt(word, n-2) && !isVowelAt(word, n-3)
}

// Step 1a: Handle plurals
func step1a(word string) string {
	if strings.HasSuffix(word, "sses") {
		return word[:len(word)-2] // sses -> ss
	}
	if strings.HasSuffix(word, "ies") {
		return word[:len(word)-2] // ies -> i
	}
	if strings.HasSuffix(word, "ss") {
		return word // ss -> ss (no change)
	}
	if strings.HasSuffix(word, "s") && len(word) > 2 {
		return word[:len(word)-1] // s -> (remove)
	}
	return word
}

// Step 1b: Handle -ed, -ing
func step1b(word string) string {
	if strings.HasSuffix(word, "eed") {
		stem := word[:len(word)-3]
		if measure(stem) > 0 {
			return stem + "ee"
		}
		return word
	}

	modified := false
	var stem string

	if strings.HasSuffix(word, "ed") {
		stem = word[:len(word)-2]
		if containsVowel(stem) {
			word = stem
			modified = true
		}
	} else if strings.HasSuffix(word, "ing") {
		stem = word[:len(word)-3]
		if containsVowel(stem) {
			word = stem
			modified = true
		}
	}

	if modified {
		if strings.HasSuffix(word, "at") || strings.HasSuffix(word, "bl") || strings.HasSuffix(word, "iz") {
			return word + "e"
		}
		if endsWithDoubleConsonant(word) {
			last := word[len(word)-1]
			if last != 'l' && last != 's' && last != 'z' {
				return word[:len(word)-1]
			}
		}
		if measure(word) == 1 && endsCVC(word) {
			return word + "e"
		}
	}

	return word
}

// Step 1c: Replace y with i
func step1c(word string) string {
	if strings.HasSuffix(word, "y") && len(word) > 2 {
		stem := word[:len(word)-1]
		if containsVowel(stem) {
			return stem + "i"
		}
	}
	return word
}

// Step 2: Map double suffixes to single ones
func step2(word string) string {
	suffixes := map[string]string{
		"ational": "ate", "tional": "tion", "enci": "ence", "anci": "ance",
		"izer": "ize", "abli": "able", "alli": "al", "entli": "ent",
		"eli": "e", "ousli": "ous", "ization": "ize", "ation": "ate",
		"ator": "ate", "alism": "al", "iveness": "ive", "fulness": "ful",
		"ousness": "ous", "aliti": "al", "iviti": "ive", "biliti": "ble",
	}

	for suffix, replacement := range suffixes {
		if strings.HasSuffix(word, suffix) {
			stem := word[:len(word)-len(suffix)]
			if measure(stem) > 0 {
				return stem + replacement
			}
			break
		}
	}
	return word
}

// Step 3: Remove derivational suffixes
func step3(word string) string {
	suffixes := map[string]string{
		"icate": "ic", "ative": "", "alize": "al", "iciti": "ic",
		"ical": "ic", "ful": "", "ness": "",
	}

	for suffix, replacement := range suffixes {
		if strings.HasSuffix(word, suffix) {
			stem := word[:len(word)-len(suffix)]
			if measure(stem) > 0 {
				return stem + replacement
			}
			break
		}
	}
	return word
}

// Step 4: Remove -ant, -ence, etc.
func step4(word string) string {
	suffixes := []string{
		"al", "ance", "ence", "er", "ic", "able", "ible", "ant", "ement",
		"ment", "ent", "ion", "ou", "ism", "ate", "iti", "ous", "ive", "ize",
	}

	for _, suffix := range suffixes {
		if strings.HasSuffix(word, suffix) {
			stem := word[:len(word)-len(suffix)]
			if suffix == "ion" {
				// Special case: stem must end in s or t
				if len(stem) > 0 && (stem[len(stem)-1] == 's' || stem[len(stem)-1] == 't') {
					if measure(stem) > 1 {
						return stem
					}
				}
			} else if measure(stem) > 1 {
				return stem
			}
			break
		}
	}
	return word
}

// Step 5: Final cleanup
func step5(word string) string {
	// Step 5a: Remove trailing e
	if strings.HasSuffix(word, "e") {
		stem := word[:len(word)-1]
		if measure(stem) > 1 {
			return stem
		}
		if measure(stem) == 1 && !endsCVC(stem) {
			return stem
		}
	}

	// Step 5b: Remove double l
	if strings.HasSuffix(word, "ll") && measure(word[:len(word)-1]) > 1 {
		return word[:len(word)-1]
	}

	return word
}
