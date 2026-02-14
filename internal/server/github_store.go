package server

// GitHubStore provides access to persisted GitHub integration state.
type GitHubStore interface {
	SetGitHubState(key, value string) error
	GetGitHubState(key string) (string, error)
	DeleteGitHubState(key string) error
}
