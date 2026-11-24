package domain

// IntegrationCredentialInfo is what a successfully authenticated integration token resolves to.
type IntegrationCredentialInfo struct {
	PrincipalID    string
	WorkspaceID    string
	AllowedDirRoot string
}
