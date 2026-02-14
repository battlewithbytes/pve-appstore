package github

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const apiBase = "https://api.github.com"

// Client is a lightweight GitHub REST API client using stdlib only.
type Client struct {
	token      string
	httpClient *http.Client
}

// NewClient creates a GitHub API client with the given OAuth token.
func NewClient(token string) *Client {
	return &Client{
		token:      token,
		httpClient: &http.Client{},
	}
}

func (c *Client) do(method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, apiBase+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}

func (c *Client) doJSON(method, path string, body interface{}, result interface{}) error {
	resp, err := c.do(method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("GitHub API %s %s: %d %s", method, path, resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
	}
	return nil
}

// User returns the authenticated user's profile.
func (c *Client) User() (*GitHubUser, error) {
	var user GitHubUser
	if err := c.doJSON("GET", "/user", nil, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// ForkRepo forks a repository. This is idempotent — if a fork already exists, GitHub returns it.
func (c *Client) ForkRepo(owner, repo string) (*ForkResult, error) {
	var raw struct {
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
	}
	path := fmt.Sprintf("/repos/%s/%s/forks", owner, repo)
	if err := c.doJSON("POST", path, map[string]string{}, &raw); err != nil {
		return nil, err
	}
	return &ForkResult{
		FullName: raw.FullName,
		CloneURL: raw.CloneURL,
		Owner:    raw.Owner.Login,
	}, nil
}

// GetDefaultBranchSHA returns the SHA of the default branch HEAD.
func (c *Client) GetDefaultBranchSHA(owner, repo string) (string, string, error) {
	// First get the repo to find the default branch name
	var repoInfo struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := c.doJSON("GET", fmt.Sprintf("/repos/%s/%s", owner, repo), nil, &repoInfo); err != nil {
		return "", "", err
	}

	branch := repoInfo.DefaultBranch
	if branch == "" {
		branch = "main"
	}

	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	path := fmt.Sprintf("/repos/%s/%s/git/ref/heads/%s", owner, repo, branch)
	if err := c.doJSON("GET", path, nil, &ref); err != nil {
		return "", "", err
	}
	return ref.Object.SHA, branch, nil
}

// CreateBranch creates a new branch from the given base SHA.
func (c *Client) CreateBranch(owner, repo, branch, baseSHA string) error {
	body := map[string]string{
		"ref": "refs/heads/" + branch,
		"sha": baseSHA,
	}
	path := fmt.Sprintf("/repos/%s/%s/git/refs", owner, repo)

	resp, err := c.do("POST", path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 422 means branch already exists — that's fine (idempotent)
	if resp.StatusCode == 422 {
		return nil
	}
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API create branch: %d %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// GetFileSHA returns the SHA of a file at a given path and branch, or "" if not found.
func (c *Client) GetFileSHA(owner, repo, path, branch string) (string, error) {
	apiPath := fmt.Sprintf("/repos/%s/%s/contents/%s?ref=%s", owner, repo, url.PathEscape(path), url.QueryEscape(branch))

	resp, err := c.do("GET", apiPath, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "", nil
	}
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API get file SHA: %d %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.SHA, nil
}

// CreateOrUpdateFile creates or updates a file in a repository.
func (c *Client) CreateOrUpdateFile(owner, repo, path, branch string, content []byte, msg, sha string) error {
	body := map[string]string{
		"message": msg,
		"content": base64.StdEncoding.EncodeToString(content),
		"branch":  branch,
	}
	if sha != "" {
		body["sha"] = sha
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, path)
	return c.doJSON("PUT", apiPath, body, nil)
}

// DeleteFile deletes a file from a repository.
func (c *Client) DeleteFile(owner, repo, path, branch, sha, msg string) error {
	body := map[string]string{
		"message": msg,
		"sha":     sha,
		"branch":  branch,
	}
	apiPath := fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, path)
	return c.doJSON("DELETE", apiPath, body, nil)
}

// CreatePullRequest opens a pull request on the upstream repository.
func (c *Client) CreatePullRequest(upstream, repo, title, body, head, base string) (*PRResult, error) {
	reqBody := map[string]string{
		"title": title,
		"body":  body,
		"head":  head,
		"base":  base,
	}
	apiPath := fmt.Sprintf("/repos/%s/%s/pulls", upstream, repo)
	var pr PRResult
	if err := c.doJSON("POST", apiPath, reqBody, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}
