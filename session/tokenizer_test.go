package session

import (
	"reflect"
	"testing"
)

func TestNewTokenizer(t *testing.T) {
	tokenizer := NewTokenizer()
	if tokenizer == nil {
		t.Fatal("NewTokenizer returned nil")
	}
	if tokenizer.stopWords == nil {
		t.Fatal("stopWords map is nil")
	}
	// Verify some common stop words are present
	expectedStopWords := []string{"the", "a", "an", "and", "or", "is", "are", "in", "on"}
	for _, word := range expectedStopWords {
		if !tokenizer.stopWords[word] {
			t.Errorf("expected stop word %q to be present", word)
		}
	}
}

func TestTokenize_BasicTokenization(t *testing.T) {
	tokenizer := NewTokenizer()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "single word",
			input:    "hello",
			expected: []string{"hello"},
		},
		{
			name:     "multiple words",
			input:    "hello world",
			expected: []string{"hello", "world"},
		},
		{
			name:     "with punctuation",
			input:    "hello, world! how are you?",
			expected: []string{"hello", "world"}, // "are", "you", "how" are stop words
		},
		{
			name:     "mixed case",
			input:    "Hello World HELLO",
			expected: []string{"hello", "world"}, // deduplicated
		},
		{
			name:     "numbers",
			input:    "test123 456test",
			expected: []string{"test123", "456test"},
		},
		{
			name:     "special characters",
			input:    "hello@world.com test-case under_score",
			expected: []string{"hello", "world", "com", "test", "case", "under", "score"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tokenizer.Tokenize(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Tokenize(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTokenize_StopWordRemoval(t *testing.T) {
	tokenizer := NewTokenizer()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "all stop words",
			input:    "the a an is are was were",
			expected: []string{},
		},
		{
			name:     "mixed stop and content words",
			input:    "the quick brown fox",
			expected: []string{"quick", "brown", "fox"},
		},
		{
			name:     "sentence with stop words",
			input:    "I am going to the store",
			expected: []string{"go", "store"}, // "going" stems to "go"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tokenizer.Tokenize(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Tokenize(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTokenize_Stemming(t *testing.T) {
	tokenizer := NewTokenizer()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "plural to singular",
			input:    "cats dogs houses",
			expected: []string{"cat", "dog", "hous"},
		},
		{
			name:     "ing suffix",
			input:    "running jumping swimming",
			expected: []string{"run", "jump", "swim"},
		},
		{
			name:     "ed suffix",
			input:    "walked talked jumped",
			expected: []string{"walk", "talk", "jump"},
		},
		{
			name:     "ies to i",
			input:    "flies tries cries",
			expected: []string{"fli", "tri", "cri"},
		},
		{
			name:     "ational to ate",
			input:    "relational conditional",
			expected: []string{"relat", "condit"}, // actual Porter stemmer output
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tokenizer.Tokenize(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Tokenize(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTokenize_Deduplication(t *testing.T) {
	tokenizer := NewTokenizer()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "exact duplicates",
			input:    "test test test",
			expected: []string{"test"},
		},
		{
			name:     "case duplicates",
			input:    "Test TEST test",
			expected: []string{"test"},
		},
		{
			name:     "stem duplicates",
			input:    "run running runs",
			expected: []string{"run"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tokenizer.Tokenize(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Tokenize(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTokenize_ShortWords(t *testing.T) {
	tokenizer := NewTokenizer()

	// Single character words should be filtered out
	result := tokenizer.Tokenize("I a x y z test")
	expected := []string{"test"} // "I" and "a" are stop words, single chars filtered
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Tokenize with short words = %v, want %v", result, expected)
	}
}

func TestTokenizeWithPositions(t *testing.T) {
	tokenizer := NewTokenizer()

	tests := []struct {
		name     string
		input    string
		expected []TokenPosition
	}{
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:  "single word",
			input: "hello",
			expected: []TokenPosition{
				{Token: "hello", Start: 0, End: 5},
			},
		},
		{
			name:  "multiple words",
			input: "hello world",
			expected: []TokenPosition{
				{Token: "hello", Start: 0, End: 5},
				{Token: "world", Start: 6, End: 11},
			},
		},
		{
			name:  "with stop words",
			input: "the quick fox",
			expected: []TokenPosition{
				{Token: "quick", Start: 4, End: 9},
				{Token: "fox", Start: 10, End: 13},
			},
		},
		{
			name:  "with punctuation",
			input: "hello, world!",
			expected: []TokenPosition{
				{Token: "hello", Start: 0, End: 5},
				{Token: "world", Start: 7, End: 12},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tokenizer.TokenizeWithPositions(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("TokenizeWithPositions(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsStopWord(t *testing.T) {
	tokenizer := NewTokenizer()

	tests := []struct {
		word     string
		expected bool
	}{
		{"the", true},
		{"a", true},
		{"and", true},
		{"hello", false},
		{"world", false},
		{"THE", true}, // case insensitive
		{"And", true}, // case insensitive
		{"test", false},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			result := tokenizer.IsStopWord(tt.word)
			if result != tt.expected {
				t.Errorf("IsStopWord(%q) = %v, want %v", tt.word, result, tt.expected)
			}
		})
	}
}

func TestPorterStem(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Short words unchanged
		{"short word a", "a", "a"},
		{"short word ab", "ab", "ab"},

		// Step 1a: plurals
		{"sses suffix", "caresses", "caress"},
		{"ies suffix", "ponies", "poni"},
		{"ss suffix", "caress", "caress"},
		{"s suffix", "cats", "cat"},

		// Step 1b: -ed, -ing
		{"eed suffix with measure", "agreed", "agre"}, // actual output for "agreed"
		{"ed suffix", "plastered", "plaster"},
		{"ing suffix", "motoring", "motor"},

		// Step 1c: y to i
		{"y to i", "happy", "happi"},

		// Common words
		{"running", "running", "run"},
		{"jumping", "jumping", "jump"},
		{"walked", "walked", "walk"},
		{"connection", "connection", "connect"},
		{"relational", "relational", "relat"},
		{"conditional", "conditional", "condit"},
		{"rational", "rational", "ration"},
		{"electricity", "electricity", "electr"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := porterStem(tt.input)
			if result != tt.expected {
				t.Errorf("porterStem(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMeasure(t *testing.T) {
	tests := []struct {
		word     string
		expected int
	}{
		{"tr", 0},
		{"ee", 0},
		{"tree", 0},
		{"y", 0},
		{"by", 0},
		{"trouble", 1},
		{"oats", 1},
		{"trees", 1},
		{"ivy", 1},
		{"troubles", 2},
		{"private", 2},
		{"oaten", 2},
		{"orrery", 2},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			result := measure(tt.word)
			if result != tt.expected {
				t.Errorf("measure(%q) = %d, want %d", tt.word, result, tt.expected)
			}
		})
	}
}

func TestContainsVowel(t *testing.T) {
	tests := []struct {
		word     string
		expected bool
	}{
		{"test", true},
		{"xyz", true},    // y is vowel when preceded by consonant (x)
		{"rhythm", true}, // y is vowel when preceded by consonant
		{"a", true},
		{"bcdfg", false},
		{"type", true},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			result := containsVowel(tt.word)
			if result != tt.expected {
				t.Errorf("containsVowel(%q) = %v, want %v", tt.word, result, tt.expected)
			}
		})
	}
}

func TestEndsWithDoubleConsonant(t *testing.T) {
	tests := []struct {
		word     string
		expected bool
	}{
		{"hopp", true},
		{"fall", true},
		{"miss", true},
		{"hop", false},
		{"hoop", false}, // oo is vowel-vowel
		{"a", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			result := endsWithDoubleConsonant(tt.word)
			if result != tt.expected {
				t.Errorf("endsWithDoubleConsonant(%q) = %v, want %v", tt.word, result, tt.expected)
			}
		})
	}
}

func TestEndsCVC(t *testing.T) {
	tests := []struct {
		word     string
		expected bool
	}{
		{"hop", true},
		{"wil", true},
		{"fil", true},
		{"how", false}, // ends in w
		{"box", false}, // ends in x
		{"bay", false}, // ends in y
		{"ab", false},  // too short
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			result := endsCVC(tt.word)
			if result != tt.expected {
				t.Errorf("endsCVC(%q) = %v, want %v", tt.word, result, tt.expected)
			}
		})
	}
}

// Benchmark tests for performance validation
func BenchmarkTokenize(b *testing.B) {
	tokenizer := NewTokenizer()
	text := "The quick brown fox jumps over the lazy dog. This is a test sentence with multiple words for tokenization performance testing."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tokenizer.Tokenize(text)
	}
}

func BenchmarkTokenizeWithPositions(b *testing.B) {
	tokenizer := NewTokenizer()
	text := "The quick brown fox jumps over the lazy dog. This is a test sentence with multiple words for tokenization performance testing."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tokenizer.TokenizeWithPositions(text)
	}
}

func BenchmarkPorterStem(b *testing.B) {
	words := []string{"running", "jumping", "connection", "relational", "electricity", "conditional"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, word := range words {
			porterStem(word)
		}
	}
}

// Test real-world content similar to Claude history messages
func TestTokenize_RealWorldContent(t *testing.T) {
	tokenizer := NewTokenizer()

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "code discussion",
			input: "Let me help you implement the authentication feature. I'll create a new function that handles user login.",
		},
		{
			name:  "error message",
			input: "Error: Failed to connect to database. Connection timeout after 30 seconds.",
		},
		{
			name:  "technical content",
			input: "The React component uses useState and useEffect hooks for state management and side effects.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tokenizer.Tokenize(tt.input)
			if len(result) == 0 {
				t.Errorf("Tokenize(%q) returned empty result", tt.input)
			}
			// Verify no stop words in result
			for _, token := range result {
				if tokenizer.IsStopWord(token) {
					t.Errorf("Result contains stop word: %q", token)
				}
			}
		})
	}
}
