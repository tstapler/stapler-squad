package services

import (
	"bytes"
	"encoding/json"
	"strconv"
	"time"
	"unicode/utf8"
)

type headerKeyID int

const (
	headerKeyOther headerKeyID = iota
	headerKeyTime
	headerKeyLevel
	headerKeyMsg
	headerKeySession
)

// parseJSONLogHeader is the cheap first pass of parseJSONLogLine: it accepts
// exactly the lines parseJSONLogLine accepts and yields the same Timestamp,
// Level and Session, but folds no message and allocates no map. To stay
// byte-for-byte equivalent to the map[string]any decode it replaces, it matches
// keys case-sensitively (an escaped key is unescaped first), lets the last
// duplicate win, treats a wrong-typed value as absent, and rejects a line
// holding a number float64 cannot represent, as json.Unmarshal into any does.
// line must be a trimmed candidate starting with '{'.
func parseJSONLogHeader(line []byte) (parsedLogLine, bool) {
	if len(line) == 0 || line[0] != '{' || !json.Valid(line) {
		return parsedLogLine{}, false
	}
	spans, ok := scanHeaderSpans(line)
	if !ok {
		return parsedLogLine{}, false
	}

	timeStr, ok := jsonStringValue(spans.time)
	if !ok || timeStr == "" {
		return parsedLogLine{}, false
	}
	if _, hasMsg := jsonStringValue(spans.msg); !hasMsg {
		return parsedLogLine{}, false
	}
	ts, err := time.Parse(time.RFC3339Nano, timeStr)
	if err != nil {
		return parsedLogLine{}, false
	}
	level, hasLevel := jsonStringValue(spans.level)
	if !hasLevel || level == "" {
		level = "INFO"
	}
	session, _ := jsonStringValue(spans.session) //nolint:errcheck // second result is "was a string", not an error
	return parsedLogLine{Timestamp: ts, Level: level, Session: session}, true
}

// headerSpans holds the raw value of the last occurrence of each header key.
type headerSpans struct{ time, level, msg, session []byte }

// scanHeaderSpans walks the top-level members of validated JSON object line.
func scanHeaderSpans(line []byte) (headerSpans, bool) {
	var spans headerSpans
	i := skipJSONSpace(line, 1)
	for line[i] != '}' {
		keyEnd := jsonStringEnd(line, i)
		key := headerKey(line[i:keyEnd])
		i = skipJSONSpace(line, skipJSONSpace(line, keyEnd)+1) // past the colon
		end, ok := jsonValueEnd(line, i)
		if !ok {
			return headerSpans{}, false
		}
		switch key {
		case headerKeyTime:
			spans.time = line[i:end]
		case headerKeyLevel:
			spans.level = line[i:end]
		case headerKeyMsg:
			spans.msg = line[i:end]
		case headerKeySession:
			spans.session = line[i:end]
		case headerKeyOther:
		}
		i = skipJSONSpace(line, end)
		if line[i] == ',' {
			i = skipJSONSpace(line, i+1)
		}
	}
	return spans, true
}

func skipJSONSpace(b []byte, i int) int {
	for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\n' || b[i] == '\r') {
		i++
	}
	return i
}

// jsonStringEnd returns the index just past the string literal starting at b[i].
// b is already validated JSON, so the literal is terminated.
func jsonStringEnd(b []byte, i int) int {
	i++
	for b[i] != '"' {
		if b[i] == '\\' {
			i++
		}
		i++
	}
	return i + 1
}

func headerKey(quoted []byte) headerKeyID {
	inner := quoted[1 : len(quoted)-1]
	if bytes.IndexByte(inner, '\\') >= 0 {
		var unescaped string
		if json.Unmarshal(quoted, &unescaped) != nil {
			return headerKeyOther
		}
		inner = []byte(unescaped)
	}
	switch string(inner) {
	case "time":
		return headerKeyTime
	case "level":
		return headerKeyLevel
	case "msg":
		return headerKeyMsg
	case "session":
		return headerKeySession
	default:
		return headerKeyOther
	}
}

// jsonValueEnd returns the index just past the value starting at b[i], or false
// if it holds a number outside float64's range.
func jsonValueEnd(b []byte, i int) (int, bool) {
	switch c := b[i]; {
	case c == '"':
		return jsonStringEnd(b, i), true
	case c == '{' || c == '[':
		return containerEnd(b, i)
	case c == '-' || (c >= '0' && c <= '9'):
		return numberEnd(b, i)
	case c == 'f':
		return i + len("false"), true
	default: // true, null
		return i + len("true"), true
	}
}

func containerEnd(b []byte, i int) (int, bool) {
	depth := 0
	for i < len(b) {
		switch c := b[i]; {
		case c == '"':
			i = jsonStringEnd(b, i)
			continue
		case c == '{' || c == '[':
			depth++
		case c == '}' || c == ']':
			depth--
			if depth == 0 {
				return i + 1, true
			}
		case c == '-' || (c >= '0' && c <= '9'):
			end, ok := numberEnd(b, i)
			if !ok {
				return 0, false
			}
			i = end
			continue
		}
		i++
	}
	return 0, false
}

func numberEnd(b []byte, i int) (int, bool) {
	j := i
	for j < len(b) && (b[j] == '+' || b[j] == '-' || b[j] == '.' || b[j] == 'e' || b[j] == 'E' || (b[j] >= '0' && b[j] <= '9')) {
		j++
	}
	_, err := strconv.ParseFloat(string(b[i:j]), 64)
	return j, err == nil
}

// jsonStringValue decodes span as a JSON string; ok is false for any other
// JSON type or an empty span.
func jsonStringValue(span []byte) (string, bool) {
	if len(span) < 2 || span[0] != '"' {
		return "", false
	}
	if bytes.IndexByte(span, '\\') < 0 && utf8.Valid(span) {
		return string(span[1 : len(span)-1]), true // no escapes: raw bytes are the value
	}
	var s string
	if json.Unmarshal(span, &s) != nil {
		return "", false
	}
	return s, true
}
