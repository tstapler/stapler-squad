package resolve

import (
	"bufio"
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Slug ports github-slugger's rules (the algorithm Starlight uses): lower-case,
// drop everything except letters, numbers, marks, connector punctuation, hyphen
// and space, then turn each space into a hyphen.
func Slug(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r == '-' || unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) || unicode.Is(unicode.Pc, r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Anchors returns every anchor a page defines, de-duplicated the way
// github-slugger does (second "install" becomes "install-1").
func Anchors(headings []string) map[string]bool {
	seen := map[string]int{}
	out := map[string]bool{}
	for _, h := range headings {
		base := Slug(h)
		slug := base
		for {
			if _, taken := out[slug]; !taken {
				break
			}
			seen[base]++
			slug = base + "-" + strconv.Itoa(seen[base])
		}
		out[slug] = true
	}
	return out
}

var (
	atxHeading = regexp.MustCompile(`^ {0,3}#{1,6}[ \t]+(.+?)[ \t]*#*[ \t]*$`)
	mdLink     = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	htmlTag    = regexp.MustCompile(`<[^>]+>`)
	inlineMark = regexp.MustCompile("[`*]")
)

// Headings extracts ATX heading texts from markdown, skipping YAML frontmatter
// and fenced code blocks, with inline markup reduced to its text.
func Headings(md []byte) []string {
	md = stripFrontmatter(md)
	var out []string
	inFence := false
	sc := bufio.NewScanner(bytes.NewReader(md))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if m := atxHeading.FindStringSubmatch(line); m != nil {
			text := mdLink.ReplaceAllString(m[1], "$1")
			text = htmlTag.ReplaceAllString(text, "")
			out = append(out, strings.TrimSpace(inlineMark.ReplaceAllString(text, "")))
		}
	}
	return out
}

func stripFrontmatter(md []byte) []byte {
	if _, body, ok := splitFrontmatter(md); ok {
		return body
	}
	return md
}

func splitFrontmatter(md []byte) (front, body []byte, ok bool) {
	s := bytes.TrimPrefix(md, []byte("\xef\xbb\xbf"))
	if !bytes.HasPrefix(s, []byte("---\n")) && !bytes.HasPrefix(s, []byte("---\r\n")) {
		return nil, md, false
	}
	rest := s[bytes.IndexByte(s, '\n')+1:]
	for off := 0; off < len(rest); {
		end := bytes.IndexByte(rest[off:], '\n')
		var line []byte
		if end < 0 {
			line = rest[off:]
		} else {
			line = rest[off : off+end]
		}
		if strings.TrimRight(string(line), "\r") == "---" {
			next := off + len(line)
			if end >= 0 {
				next++
			}
			return rest[:off], rest[next:], true
		}
		if end < 0 {
			break
		}
		off += end + 1
	}
	return nil, md, false
}
