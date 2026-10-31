package ent

type EscapeEventClient struct{}
type SessionClient struct{}

// Client mirrors the generated client: one field per entity.
type Client struct {
	EscapeEvent *EscapeEventClient
	Session     *SessionClient
}

// Tx mirrors the generated transaction: the same entity fields.
type Tx struct {
	EscapeEvent *EscapeEventClient
	Session     *SessionClient
}
