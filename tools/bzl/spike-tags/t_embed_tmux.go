//go:build embed_tmux

package tags

import _ "embed"

//go:embed embed/tmux
var tmuxEmbedded []byte

func init() { register("embed_tmux:" + string(tmuxEmbedded)) }
