package session

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/tstapler/stapler-squad/config"
)

// attachedImageRe matches the markdown BacklogItemForm inserts for an uploaded
// image: ![name](/api/local/serve/<abs path>). encodeURI leaves parentheses
// unescaped, so one level of balanced parens is allowed inside the path.
var attachedImageRe = regexp.MustCompile(`!\[[^\n]*?\]\(/api/local/serve(/(?:[^()\s]|\([^()\s]*\))+)\)`)

// attachedImageExts mirrors the types backlog_attachment_upload_handler.go accepts.
var attachedImageExts = map[string]bool{".png": true, ".jpg": true, ".gif": true, ".webp": true}

// WriteDescriptionSection is the single place a backlog item's description is
// rendered into an agent prompt: a "## Description" section (HTML-stripped and
// capped at maxLen; maxLen <= 0 means verbatim) followed by the item's attached
// images as file paths. The image list is built from the uncapped description so
// truncation can't cut off an image link.
func WriteDescriptionSection(sb *strings.Builder, description string, maxLen int) {
	body := description
	if maxLen > 0 {
		body = sanitizeField(description, maxLen)
	}
	fmt.Fprintf(sb, "## Description\n%s\n\n", body)
	sb.WriteString(AttachedImagesSection(description))
}

// AttachedImagesSection lists description-embedded images as absolute paths, or
// returns "" when there are none. Use it directly only where a prompt has its own
// description layout; prefer WriteDescriptionSection.
func AttachedImagesSection(description string) string {
	return attachedImagesSection(description, backlogAttachmentDir())
}

// attachedImagesSection: the /api/local/serve URL only resolves through the web
// server, so the agent needs real paths. Descriptions can come from untrusted
// sources (synced GitHub issues), so only image files under attachDir are listed,
// and paths with control characters (prompt injection) are dropped.
func attachedImagesSection(description, attachDir string) string {
	if attachDir == "" {
		return ""
	}
	root := filepath.Clean(attachDir) + string(filepath.Separator)
	seen := map[string]bool{}
	var paths []string
	for _, m := range attachedImageRe.FindAllStringSubmatch(description, -1) {
		p, err := url.PathUnescape(m[1])
		if err != nil || strings.ContainsFunc(p, isControlOrLineBreak) {
			continue
		}
		p = filepath.Clean(p)
		if !strings.HasPrefix(p, root) || !attachedImageExts[strings.ToLower(filepath.Ext(p))] || seen[p] {
			continue
		}
		seen[p] = true
		paths = append(paths, p)
	}
	if len(paths) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Attached Images\nThe description references these images; view them with the Read tool:\n")
	for _, p := range paths {
		fmt.Fprintf(&sb, "- %s\n", p)
	}
	sb.WriteString("\n")
	return sb.String()
}

func isControlOrLineBreak(r rune) bool {
	return unicode.IsControl(r) || r == '\u2028' || r == '\u2029'
}

// backlogAttachmentDir returns "" when the config dir can't be resolved, which
// disables the attached-images section rather than failing prompt construction.
func backlogAttachmentDir() string {
	dir, err := config.GetConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, config.BacklogAttachmentDirName)
}
