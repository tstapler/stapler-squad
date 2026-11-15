// Package webhookstore persists webhook-management registrations and their request
// ledger. session.EntRepository embeds *Store so existing callers keep their method set.
package webhookstore

import "github.com/tstapler/stapler-squad/session/ent"

// Store owns the webhook registration, workflow-provenance and request-ledger tables.
type Store struct {
	client *ent.Client
}

// New returns a Store over client, which must already hold the migrated schema.
func New(client *ent.Client) *Store { return &Store{client: client} }
