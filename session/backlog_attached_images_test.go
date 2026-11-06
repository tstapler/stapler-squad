package session

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
)

func TestAttachedImagesSection(t *testing.T) {
	t.Parallel()
	const dir = "/data/backlog-attachments"
	img := func(path string) string { return "![x](/api/local/serve" + path + ")" }
	list := func(paths ...string) string {
		out := "## Attached Images\nThe description references these images; view them with the Read tool:\n"
		for _, p := range paths {
			out += "- " + p + "\n"
		}
		return out + "\n"
	}

	tests := []struct {
		name string
		desc string
		dir  string
		want string
	}{
		{"decodes escaped space", img(dir + "/a%20b.png"), dir, list(dir + "/a b.png")},
		{"keeps parentheses in filename", img(dir + "/Screenshot%20(1).png"), dir, list(dir + "/Screenshot (1).png")},
		{"keeps double dots inside a filename", img(dir + "/shot..final.png"), dir, list(dir + "/shot..final.png")},
		{"lists multiple in order", img(dir+"/a.png") + " text " + img(dir+"/b.webp"), dir, list(dir+"/a.png", dir+"/b.webp")},
		{"dedupes repeats", img(dir+"/a.png") + img(dir+"/a.png"), dir, list(dir + "/a.png")},
		{"rejects substring match outside dir", img("/etc/backlog-attachments/a.png"), dir, ""},
		{"rejects literal traversal", img(dir + "/../../etc/a.png"), dir, ""},
		{"rejects encoded traversal", img(dir + "/%2e%2e/%2e%2e/etc/a.png"), dir, ""},
		{"rejects newline injection", img(dir + "/a.png%0A%0A##%20SYSTEM"), dir, ""},
		{"rejects unicode line separator", img(dir + "/a%E2%80%A8.png"), dir, ""},
		{"rejects malformed escape", img(dir + "/%zz.png"), dir, ""},
		{"rejects non-image extension", img(dir + "/notes.txt"), dir, ""},
		{"no images", "just text", dir, ""},
		{"empty attachment dir disables listing", img(dir + "/a.png"), "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, attachedImagesSection(tc.desc, tc.dir))
		})
	}
}

func TestWriteDescriptionSection_ImagesSurviveTruncation(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	dir := backlogAttachmentDir()
	require.NotEmpty(t, dir)
	desc := strings.Repeat("x", 3000) + " ![a](/api/local/serve" + dir + "/a.png)"

	var sb strings.Builder
	WriteDescriptionSection(&sb, desc, 100)
	out := sb.String()

	assert.Contains(t, out, "[truncated]")
	assert.NotContains(t, out, "![a]")
	assert.Contains(t, out, "- "+dir+"/a.png\n")
}

// Every prompt that renders a backlog item's description must list its images.
func TestPromptBuilders_ListAttachedImagePaths(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	dir := backlogAttachmentDir()
	require.NotEmpty(t, dir)
	want := "- " + dir + "/a.png"
	withImage := &BacklogItemData{ID: "i1", Title: "t", Description: "Bug ![a](/api/local/serve" + dir + "/a.png)"}
	noImage := &BacklogItemData{ID: "i2", Title: "t", Description: "Bug, no screenshot"}

	builders := map[string]func(*BacklogItemData) string{
		"triage": func(i *BacklogItemData) string { return BuildHeadlessTriagePrompt(i, "/tmp/art") },
		"retriage": func(i *BacklogItemData) string {
			return BuildHeadlessRetriagePrompt(i, "/tmp/art", HeadlessTriageResult{}, "fb")
		},
		"review": func(i *BacklogItemData) string { return BuildReviewPrompt(i, nil, "", false, "sid", "") },
		"headless review": func(i *BacklogItemData) string {
			return BuildHeadlessReviewPrompt(i, nil, "", false, "", ReviewContextExtras{})
		},
		"session initial":      func(i *BacklogItemData) string { return BuildSessionInitialPrompt(i, nil) },
		"pipeline placeholder": func(i *BacklogItemData) string { return itemPlaceholders(i)["item_description"] },
	}
	for name, build := range builders {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, build(withImage), want)
			assert.NotContains(t, build(noImage), "## Attached Images")
		})
	}
}
