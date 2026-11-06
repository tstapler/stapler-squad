package services

import "unicode/utf8"

// metaToolInputUnrenderable tells the card to hide Approve and offer Open session:
// the operator could not see what they would allow.
const metaToolInputUnrenderable = "tool_input_unrenderable"

// toolInputUnrenderable reports an input a text row cannot show faithfully: a
// command or path that is not clean text (invalid UTF-8, NUL or a control
// character other than tab and newline), or a non-empty input with no string
// field at all to display. An empty input hides nothing.
func toolInputUnrenderable(input map[string]interface{}) bool {
	if len(input) == 0 {
		return false
	}
	hasText := false
	for _, v := range input {
		if str, ok := v.(string); ok {
			hasText = true
			if !cleanText(str) {
				return true
			}
		}
	}
	return !hasText
}

func cleanText(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' || r == 0x7f {
			return false
		}
	}
	return true
}
