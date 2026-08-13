package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// MiscSessionHibernate describes the HibernateSession RPC.
var MiscSessionHibernate = featureregistry.Feature{
	ID:          "hibernate-session",
	Title:       "Hibernate Session",
	Description: "Hibernates a running session, persisting its state and suspending its process.",
	RPCIDs:      []string{"HibernateSession"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// MiscSessionResumeHibernated describes the ResumeHibernatedSession RPC.
var MiscSessionResumeHibernated = featureregistry.Feature{
	ID:          "resume-hibernated-session",
	Title:       "Resume Hibernated Session",
	Description: "Resumes a previously hibernated session, restoring its process from saved state.",
	RPCIDs:      []string{"ResumeHibernatedSession"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// MiscSessionListTokens describes the ListSessionTokens RPC.
var MiscSessionListTokens = featureregistry.Feature{
	ID:          "list-session-tokens",
	Title:       "List Session Tokens",
	Description: "Lists token usage records associated with a session for cost tracking.",
	RPCIDs:      []string{"ListSessionTokens"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// MiscSessionWrite describes the WriteToSession RPC.
var MiscSessionWrite = featureregistry.Feature{
	ID:          "write-to-session",
	Title:       "Write To Session",
	Description: "Writes input text directly to a session terminal as if typed by the user.",
	RPCIDs:      []string{"WriteToSession"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(MiscSessionHibernate)
	featureregistry.Register(MiscSessionResumeHibernated)
	featureregistry.Register(MiscSessionListTokens)
	featureregistry.Register(MiscSessionWrite)
}
