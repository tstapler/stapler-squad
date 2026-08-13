package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

var HistoryGetDetail = featureregistry.Feature{
	ID:          "history-get-detail",
	Title:       "Get Claude History Detail",
	Description: "Retrieves the full detail of a single Claude conversation history entry by ID.",
	RPCIDs:      []string{"history:get-detail"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

var HistoryGetMessages = featureregistry.Feature{
	ID:          "history-get-messages",
	Title:       "Get Claude History Messages",
	Description: "Retrieves the individual messages from a Claude conversation history entry.",
	RPCIDs:      []string{"history:get-messages"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

var HistoryList = featureregistry.Feature{
	ID:          "history-list",
	Title:       "List Claude History",
	Description: "Lists all available Claude conversation history entries.",
	RPCIDs:      []string{"history:list"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

var HistorySearch = featureregistry.Feature{
	ID:          "history-search",
	Title:       "Search Claude History",
	Description: "Searches Claude conversation history entries by query string.",
	RPCIDs:      []string{"history:search"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(HistoryGetDetail)
	featureregistry.Register(HistoryGetMessages)
	featureregistry.Register(HistoryList)
	featureregistry.Register(HistorySearch)
}
