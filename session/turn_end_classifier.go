package session

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// TurnEndClass is the heuristic verdict for an agent's last message.
type TurnEndClass string

const (
	TurnEndClassFinished TurnEndClass = "finished"
	TurnEndClassQuestion TurnEndClass = "question"
)

// TurnEndConfidence gates escalation: only High yields TurnEndClassQuestion without a tiebreak.
type TurnEndConfidence int

const (
	TurnEndConfidenceLow TurnEndConfidence = iota
	TurnEndConfidenceMedium
	TurnEndConfidenceHigh
)

// TurnEndClassification is the result of ClassifyTurnEnd.
type TurnEndClassification struct {
	Class      TurnEndClass
	Confidence TurnEndConfidence
}

// NeedsTiebreak reports an ambiguous Finished verdict a later LLM tiebreak may upgrade.
func (c TurnEndClassification) NeedsTiebreak() bool {
	return c.Class == TurnEndClassFinished && c.Confidence == TurnEndConfidenceMedium
}

const questionExcerptMax = 160

var (
	fencedCodeRe  = regexp.MustCompile("(?s)```.*?(?:```|$)")
	inlineCodeRe  = regexp.MustCompile("`[^`\n]*`")
	doubleQuoteRe = regexp.MustCompile(`"[^"\n]*"|“[^”\n]*”`)
	blockquoteRe  = regexp.MustCompile(`(?m)^[ \t]*>.*$`)
	ansiRe        = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)
	secretRe      = regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{20,}|sk-[A-Za-z0-9_-]{20,}|AKIA[0-9A-Z]{16}|xox[baprs]-[A-Za-z0-9-]{10,})\b`)
	spaceRe       = regexp.MustCompile(`\s+`)
	// Abbreviations whose trailing period is not a sentence end.
	abbrevRe = regexp.MustCompile(`(?i)(?:\be\.g|\bi\.e|\bvs|\betc|\bcf|\bno)\.$`)

	// directedCueRe: the question is addressed to the operator (a decision, permission, or input).
	directedCueRe = regexp.MustCompile(`(?i)\b(?:` +
		`do you|would you|could you|can you|will you|are you (?:ok|okay|happy|fine)|have you|did you mean|` +
		`should i|shall i|should we|shall we|can i|may i|could i|would it be ok|` +
		`want me to|like me to|you want|you like|you prefer|you rather|` +
		`which (?:one|option|approach|branch|environment|of)|what (?:should|would|do you|timeout|version)|` +
		`how (?:should|would|do you)|(?:ok|okay|fine) (?:to|if|with)|sound good|go ahead|proceed|` +
		`anything else|what next|work on next|prefer|(?:supposed|meant) to|you|your` +
		`)\b`)
	// directedStartRe: bare interrogative openers addressed to the operator.
	directedStartRe = regexp.MustCompile(`(?i)^(?:want|ready|shall|should|would|could|can|do|does the|is it (?:ok|okay|fine)|is that (?:ok|okay|fine|acceptable)|are you|which|ok(?:ay)? to|proceed|continue)\b`)
	// rhetoricalStartRe: self-answering lead-ins that are not addressed to the operator.
	rhetoricalStartRe = regexp.MustCompile(`(?i)^(?:why|how come|so what|what does (?:this|that) mean|who knows|wondering|you might wonder|perhaps|maybe)\b`)
)

// ClassifyTurnEnd decides whether a final agent message is a question awaiting the operator.
// Pure and bounded: uncertain input yields Finished. Only Claude is trusted; other programs
// are Finished until they have a per-program corpus. Decision uses sentence structure only,
// never imperative text inside the message.
func ClassifyTurnEnd(program, text string) TurnEndClassification {
	finished := TurnEndClassification{Class: TurnEndClassFinished, Confidence: TurnEndConfidenceHigh}
	if !isClaudeProgram(program) {
		return finished
	}
	sentences := splitSentences(stripNonProse(text))
	if len(sentences) == 0 {
		return finished
	}
	last := sentences[len(sentences)-1]
	if !strings.HasSuffix(last, "?") {
		if isOfferCloser(last) {
			return TurnEndClassification{Class: TurnEndClassFinished, Confidence: TurnEndConfidenceMedium}
		}
		return finished
	}
	return classifyQuestionSentence(last)
}

func classifyQuestionSentence(s string) TurnEndClassification {
	if rhetoricalStartRe.MatchString(s) {
		return TurnEndClassification{Class: TurnEndClassFinished, Confidence: TurnEndConfidenceMedium}
	}
	if directedStartRe.MatchString(s) || directedCueRe.MatchString(s) || isChoiceQuestion(s) {
		return TurnEndClassification{Class: TurnEndClassQuestion, Confidence: TurnEndConfidenceHigh}
	}
	return TurnEndClassification{Class: TurnEndClassFinished, Confidence: TurnEndConfidenceMedium}
}

// isChoiceQuestion matches "X or Y?" with a short, bounded shape so a rhetorical aside is not caught.
func isChoiceQuestion(s string) bool {
	return len(s) <= questionExcerptMax && strings.Contains(strings.ToLower(s), " or ")
}

// isOfferCloser matches "let me know if..." style closers that invite but do not block.
func isOfferCloser(s string) bool {
	l := strings.ToLower(s)
	for _, p := range []string{"let me know", "feel free", "happy to", "tell me if", "just say", "if you want", "if you'd like", "if you would like"} {
		if strings.Contains(l, p) {
			return true
		}
	}
	return false
}

func isClaudeProgram(program string) bool {
	fields := strings.Fields(program)
	if len(fields) == 0 {
		return false
	}
	return strings.EqualFold(filepath.Base(fields[0]), "claude")
}

// stripNonProse removes code, quotes and blockquotes so a "?" inside them can't decide the verdict.
func stripNonProse(text string) string {
	text = fencedCodeRe.ReplaceAllString(text, "\n")
	text = blockquoteRe.ReplaceAllString(text, "")
	text = inlineCodeRe.ReplaceAllString(text, "")
	text = doubleQuoteRe.ReplaceAllString(text, "")
	return text
}

// splitSentences splits on terminal punctuation followed by whitespace, and on newlines
// (list items and paragraphs are separate sentences). Markdown emphasis/emoji are trimmed.
func splitSentences(text string) []string {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		line = cleanSentence(line)
		if line == "" {
			continue
		}
		out = append(out, splitLine(line)...)
	}
	return out
}

func splitLine(line string) []string {
	var out []string
	start := 0
	runes := []rune(line)
	for i, r := range runes {
		if r != '.' && r != '!' && r != '?' {
			continue
		}
		atEnd := i == len(runes)-1
		if !atEnd && !unicode.IsSpace(runes[i+1]) {
			continue
		}
		if r == '.' && abbrevRe.MatchString(string(runes[start:i+1])) {
			continue
		}
		if s := cleanSentence(string(runes[start : i+1])); s != "" {
			out = append(out, s)
		}
		start = i + 1
	}
	if start < len(runes) {
		if s := cleanSentence(string(runes[start:])); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// cleanSentence trims whitespace, markdown emphasis, control characters and trailing symbols/emoji
// after the terminal punctuation.
func cleanSentence(s string) string {
	s = ansiRe.ReplaceAllString(s, "")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\t' || r == '\u200b' || r == '\ufeff' {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	s = strings.TrimLeft(s, "*_#-•> \t0123456789.)")
	s = strings.TrimRightFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || r == '*' || r == '_' || unicode.IsSymbol(r) || unicode.Is(unicode.So, r)
	})
	// Close-out markers such as a stray "**" after the final "?".
	s = strings.TrimRight(s, "*_ \t")
	return s
}

// ExtractQuestionSentence returns the excerpt to store/display: the last sentence ending in "?",
// else the last sentence (tail-biased), capped at 160 chars at a word boundary and scrubbed.
func ExtractQuestionSentence(text string) string {
	sentences := splitSentences(stripNonProse(text))
	if len(sentences) == 0 {
		return ""
	}
	pick := sentences[len(sentences)-1]
	for i := len(sentences) - 1; i >= 0; i-- {
		if strings.HasSuffix(sentences[i], "?") {
			pick = sentences[i]
			break
		}
	}
	return capTail(scrubExcerpt(pick), questionExcerptMax)
}

func scrubExcerpt(s string) string {
	s = secretRe.ReplaceAllString(s, "[redacted]")
	return strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
}

// capTail keeps the last max bytes (the question usually closes the sentence) starting at a word boundary.
func capTail(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := len(s) - max
	for cut < len(s) && !utf8.RuneStart(s[cut]) {
		cut++
	}
	tail := s[cut:]
	if i := strings.IndexByte(tail, ' '); i >= 0 && i < len(tail)-1 {
		tail = tail[i+1:]
	}
	return tail
}
