//go:build embed_tymux && !embed_tmux

package tags

// Compile-time enforcement of "embed_tymux implies embed_tmux" (S7 check).
var _ = embedTymuxRequiresEmbedTmux
