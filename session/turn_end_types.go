package session

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// TurnEndSource says which path observed the end of an agent turn. The zero value is invalid:
// only the From* constructors build a TurnEnded, and each sets a real source.
type TurnEndSource string

const (
	TurnEndSourceHook       TurnEndSource = "hook"
	TurnEndSourceScreen     TurnEndSource = "screen"
	TurnEndSourceTranscript TurnEndSource = "transcript"
)

// TurnEndKind classifies what the agent's last turn asked of the operator.
type TurnEndKind uint8

const (
	TurnEndKindUnknown TurnEndKind = iota // zero value: no signal
	TurnEndKindQuestion
	TurnEndKindFinished
	TurnEndKindError
)

func (k TurnEndKind) String() string {
	switch k {
	case TurnEndKindQuestion:
		return "question"
	case TurnEndKindFinished:
		return "finished"
	case TurnEndKindError:
		return "error"
	}
	return "unknown"
}

// ParseTurnEndKind inverts TurnEndKind.String; anything unrecognised is Unknown.
func ParseTurnEndKind(s string) TurnEndKind {
	switch s {
	case "question":
		return TurnEndKindQuestion
	case "finished":
		return TurnEndKindFinished
	case "error":
		return TurnEndKindError
	}
	return TurnEndKindUnknown
}

// TurnEndConfidence is how sure the classifier is; only High yields Question without a tiebreak.
type TurnEndConfidence uint8

const (
	TurnEndConfidenceLow TurnEndConfidence = iota
	TurnEndConfidenceMedium
	TurnEndConfidenceHigh
)

func (c TurnEndConfidence) String() string {
	switch c {
	case TurnEndConfidenceMedium:
		return "medium"
	case TurnEndConfidenceHigh:
		return "high"
	}
	return "low"
}

// ParseTurnEndConfidence inverts TurnEndConfidence.String; anything unrecognised is Low.
func ParseTurnEndConfidence(s string) TurnEndConfidence {
	switch s {
	case "medium":
		return TurnEndConfidenceMedium
	case "high":
		return TurnEndConfidenceHigh
	}
	return TurnEndConfidenceLow
}

// EpisodeID identifies one turn-end episode: hash(sessionID, turnEndedAt). It is the ack and
// dedup key, so a session's repeated reports of the same turn end share it.
type EpisodeID string

// NewEpisodeID derives the stable id for a session's turn end at the given instant.
func NewEpisodeID(sessionID string, at time.Time) EpisodeID {
	sum := sha256.Sum256([]byte(sessionID + "\x00" + strconv.FormatInt(at.UnixNano(), 10)))
	return EpisodeID(hex.EncodeToString(sum[:8]))
}

func (e EpisodeID) String() string { return string(e) }

// MaxTurnEndExcerptChars bounds TurnEnded.Excerpt (runes).
const MaxTurnEndExcerptChars = 160

// TurnEnded is the immutable fact that an agent turn ended. Build it only with FromHook,
// FromScreen or FromTranscript so Source and Excerpt invariants always hold.
type TurnEnded struct {
	Episode    EpisodeID
	Kind       TurnEndKind
	Confidence TurnEndConfidence
	Excerpt    string // scrubbed, at most MaxTurnEndExcerptChars runes, the tail of the message
	Source     TurnEndSource
	At         time.Time
}

// IsZero reports whether no turn end has been recorded.
func (t TurnEnded) IsZero() bool { return t.Episode == "" }

// FromHook builds a turn end observed through the Stop hook.
func FromHook(sessionID, text string, at time.Time, kind TurnEndKind, conf TurnEndConfidence) TurnEnded {
	return newTurnEnded(sessionID, text, at, kind, conf, TurnEndSourceHook)
}

// FromScreen builds a turn end inferred from the terminal screen.
func FromScreen(sessionID, text string, at time.Time, kind TurnEndKind, conf TurnEndConfidence) TurnEnded {
	return newTurnEnded(sessionID, text, at, kind, conf, TurnEndSourceScreen)
}

// FromTranscript builds a turn end read from the transcript tail (fallback F1).
func FromTranscript(sessionID, text string, at time.Time, kind TurnEndKind, conf TurnEndConfidence) TurnEnded {
	return newTurnEnded(sessionID, text, at, kind, conf, TurnEndSourceTranscript)
}

func newTurnEnded(sessionID, text string, at time.Time, kind TurnEndKind, conf TurnEndConfidence, src TurnEndSource) TurnEnded {
	return TurnEnded{
		Episode:    NewEpisodeID(sessionID, at),
		Kind:       kind,
		Confidence: conf,
		Excerpt:    scrubExcerpt(text),
		Source:     src,
		At:         at,
	}
}

// turnEndScrubInputBytes caps the text scanned, so a huge message costs bounded work.
const turnEndScrubInputBytes = 8 << 10

var secretLikeRE = regexp.MustCompile(`(?i)\b(?:sk-[a-z0-9_-]{16,}|gh[pousr]_[a-z0-9]{20,}|xox[abprs]-[a-z0-9-]{10,}|AKIA[0-9A-Z]{16}|bearer\s+[a-z0-9._~+/=-]{16,})|(?i)\b(?:password|passwd|secret|token|api[_-]?key)\s*[:=]\s*\S+`)

// scrubExcerpt strips terminal escapes and control characters, redacts secret-looking
// tokens, collapses whitespace and keeps the last MaxTurnEndExcerptChars runes (a question
// sits at the end of a message).
func scrubExcerpt(text string) string {
	if len(text) > turnEndScrubInputBytes {
		text = text[len(text)-turnEndScrubInputBytes:]
		for len(text) > 1 && !utf8.RuneStart(text[0]) {
			text = text[1:]
		}
	}
	text = stripANSI([]byte(text))
	text = secretLikeRE.ReplaceAllString(text, "[redacted]")
	text = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		if unicode.IsControl(r) || r == utf8.RuneError {
			return -1
		}
		return r
	}, text)
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= MaxTurnEndExcerptChars {
		return text
	}
	r := []rune(text)
	return "…" + string(r[len(r)-(MaxTurnEndExcerptChars-1):])
}

// provisionalClassify is a stand-in until ClassifyTurnEnd (Story 3.1.2) replaces it: a message
// whose last sentence ends in '?' is a High-confidence question, anything else Finished/Low.
func provisionalClassify(text string) (TurnEndKind, TurnEndConfidence) {
	if strings.HasSuffix(strings.TrimSpace(text), "?") {
		return TurnEndKindQuestion, TurnEndConfidenceHigh
	}
	return TurnEndKindFinished, TurnEndConfidenceLow
}
