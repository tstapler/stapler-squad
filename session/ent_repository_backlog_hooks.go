package session

import "github.com/tstapler/stapler-squad/session/domain/backlogdto"

func (r *EntRepository) dispatchChainFire(item *backlogdto.BacklogItemData) {
	r.Backlog.Hooks().DispatchChainFire(item)
}
