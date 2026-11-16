package session

import "github.com/tstapler/stapler-squad/session/domain/backlogdto"

// publishItemChangedSnapshot publishes item as-is, without eager-loading its
// sessions: DeleteBacklogItem must snapshot before deleting, and AppendActivityNote's
// event never carries a full item (see publishItemChanged for the eager-loading path).
func (r *EntRepository) publishItemChangedSnapshot(item *backlogdto.BacklogItemData, change backlogdto.BacklogItemChange) {
	r.Backlog.Hooks().PublishItemChanged(item, change)
}

func (r *EntRepository) dispatchChainFire(item *backlogdto.BacklogItemData) {
	r.Backlog.Hooks().DispatchChainFire(item)
}
