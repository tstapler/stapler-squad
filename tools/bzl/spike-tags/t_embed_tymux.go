//go:build embed_tymux

package tags

import _ "embed"

//go:embed embed/tymuxd
var tymuxdEmbedded []byte

func init() { register("embed_tymux:" + string(tymuxdEmbedded)) }
