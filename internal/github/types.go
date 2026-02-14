package github

// GitHubUser represents a GitHub user profile.
type GitHubUser struct {
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

// ForkResult represents the result of forking a repository.
type ForkResult struct {
	FullName string `json:"full_name"`
	CloneURL string `json:"clone_url"`
	Owner    string `json:"owner"`
}

// BranchInfo represents a branch on a repository.
type BranchInfo struct {
	Name string `json:"name"`
}

// PRResult represents a created pull request.
type PRResult struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
	State   string `json:"state"`
}

