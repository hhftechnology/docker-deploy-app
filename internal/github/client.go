package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client handles GitHub API interactions
type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

// Repository represents a GitHub repository
type Repository struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	HTMLURL     string `json:"html_url"`
	CloneURL    string `json:"clone_url"`
	DefaultBranch string `json:"default_branch"`
	Private     bool   `json:"private"`
	Fork        bool   `json:"fork"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	Language    string `json:"language"`
	Size        int    `json:"size"`
	StarCount   int    `json:"stargazers_count"`
	Topics      []string `json:"topics"`
}

// FileContent represents a file from GitHub API
type FileContent struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	SHA         string `json:"sha"`
	Size        int    `json:"size"`
	URL         string `json:"url"`
	HTMLURL     string `json:"html_url"`
	GitURL      string `json:"git_url"`
	DownloadURL string `json:"download_url"`
	Type        string `json:"type"`
	Content     string `json:"content"`
	Encoding    string `json:"encoding"`
}

// User represents a GitHub user
type User struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
	Type      string `json:"type"`
}

// NewClient creates a new GitHub client
func NewClient(token string) *Client {
	return &Client{
		token:   token,
		baseURL: "https://api.github.com",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetUser gets the authenticated user information
func (c *Client) GetUser() (*User, error) {
	var user User
	err := c.makeRequest("GET", "/user", nil, &user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// ListRepositories lists repositories for the authenticated user
func (c *Client) ListRepositories(page, perPage int) ([]*Repository, error) {
	url := fmt.Sprintf("/user/repos?page=%d&per_page=%d&sort=updated", page, perPage)
	
	var repos []*Repository
	err := c.makeRequest("GET", url, nil, &repos)
	if err != nil {
		return nil, err
	}
	
	return repos, nil
}

// GetRepository gets a specific repository
func (c *Client) GetRepository(owner, repo string) (*Repository, error) {
	url := fmt.Sprintf("/repos/%s/%s", owner, repo)
	
	var repository Repository
	err := c.makeRequest("GET", url, nil, &repository)
	if err != nil {
		return nil, err
	}
	
	return &repository, nil
}

// GetFileContent gets content of a file from repository
func (c *Client) GetFileContent(owner, repo, path, ref string) (*FileContent, error) {
	url := fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, path)
	if ref != "" {
		url += "?ref=" + ref
	}
	
	var content FileContent
	err := c.makeRequest("GET", url, nil, &content)
	if err != nil {
		return nil, err
	}
	
	return &content, nil
}

// GetRawFileContent gets raw content of a file
func (c *Client) GetRawFileContent(owner, repo, path, ref string) ([]byte, error) {
	url := fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, path)
	if ref != "" {
		url += "?ref=" + ref
	}
	
	// First get file info
	content, err := c.GetFileContent(owner, repo, path, ref)
	if err != nil {
		return nil, err
	}
	
	// Download raw content
	if content.DownloadURL != "" {
		return c.downloadFile(content.DownloadURL)
	}
	
	return nil, fmt.Errorf("no download URL available")
}

// ListBranches lists branches for a repository
func (c *Client) ListBranches(owner, repo string) ([]string, error) {
	url := fmt.Sprintf("/repos/%s/%s/branches", owner, repo)
	
	var branches []struct {
		Name string `json:"name"`
	}
	
	err := c.makeRequest("GET", url, nil, &branches)
	if err != nil {
		return nil, err
	}
	
	var branchNames []string
	for _, branch := range branches {
		branchNames = append(branchNames, branch.Name)
	}
	
	return branchNames, nil
}

// SearchRepositories searches for repositories
func (c *Client) SearchRepositories(query string, page, perPage int) ([]*Repository, error) {
	url := fmt.Sprintf("/search/repositories?q=%s&page=%d&per_page=%d", query, page, perPage)
	
	var response struct {
		Items []*Repository `json:"items"`
	}
	
	err := c.makeRequest("GET", url, nil, &response)
	if err != nil {
		return nil, err
	}
	
	return response.Items, nil
}

// CheckFileExists checks if a file exists in repository
func (c *Client) CheckFileExists(owner, repo, path, ref string) (bool, error) {
	url := fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, path)
	if ref != "" {
		url += "?ref=" + ref
	}
	
	req, err := http.NewRequest("HEAD", c.baseURL+url, nil)
	if err != nil {
		return false, err
	}
	
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	
	return resp.StatusCode == 200, nil
}

// ValidateToken validates the GitHub token
func (c *Client) ValidateToken() error {
	_, err := c.GetUser()
	return err
}

// GetRateLimit gets current rate limit status
func (c *Client) GetRateLimit() (map[string]interface{}, error) {
	var rateLimit map[string]interface{}
	err := c.makeRequest("GET", "/rate_limit", nil, &rateLimit)
	return rateLimit, err
}

// makeRequest makes a request to GitHub API
func (c *Client) makeRequest(method, endpoint string, body io.Reader, target interface{}) error {
	req, err := http.NewRequest(method, c.baseURL+endpoint, body)
	if err != nil {
		return err
	}
	
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "docker-deploy-app/1.0")
	
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error: %d %s", resp.StatusCode, string(bodyBytes))
	}
	
	if target != nil {
		return json.NewDecoder(resp.Body).Decode(target)
	}
	
	return nil
}

// downloadFile downloads a file from URL
func (c *Client) downloadFile(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Authorization", "token "+c.token)
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to download file: %d", resp.StatusCode)
	}
	
	return io.ReadAll(resp.Body)
}

// ParseRepoURL parses GitHub repository URL
func ParseRepoURL(repoURL string) (owner, repo string, err error) {
	// Handle different URL formats
	repoURL = strings.TrimSpace(repoURL)
	repoURL = strings.TrimSuffix(repoURL, ".git")
	
	if strings.HasPrefix(repoURL, "https://github.com/") {
		path := strings.TrimPrefix(repoURL, "https://github.com/")
		parts := strings.Split(path, "/")
		if len(parts) >= 2 {
			return parts[0], parts[1], nil
		}
	} else if strings.HasPrefix(repoURL, "git@github.com:") {
		path := strings.TrimPrefix(repoURL, "git@github.com:")
		parts := strings.Split(path, "/")
		if len(parts) >= 2 {
			return parts[0], parts[1], nil
		}
	} else if strings.Contains(repoURL, "/") {
		// Assume it's in format "owner/repo"
		parts := strings.Split(repoURL, "/")
		if len(parts) >= 2 {
			return parts[0], parts[1], nil
		}
	}
	
	return "", "", fmt.Errorf("invalid repository URL format: %s", repoURL)
}

// IsDockerComposeRepo checks if repository contains docker-compose files
func (c *Client) IsDockerComposeRepo(owner, repo string) (bool, error) {
	// Check for common docker-compose file names
	composeFiles := []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
	}
	
	for _, file := range composeFiles {
		exists, err := c.CheckFileExists(owner, repo, file, "")
		if err != nil {
			continue
		}
		if exists {
			return true, nil
		}
	}
	
	return false, nil
}

// GetTemplateConfig gets template configuration file
func (c *Client) GetTemplateConfig(owner, repo, ref string) (map[string]interface{}, error) {
	configFiles := []string{
		".template.json",
		"template.json",
		".docker-deploy.json",
	}
	
	for _, configFile := range configFiles {
		exists, err := c.CheckFileExists(owner, repo, configFile, ref)
		if err != nil || !exists {
			continue
		}
		
		content, err := c.GetRawFileContent(owner, repo, configFile, ref)
		if err != nil {
			continue
		}
		
		var config map[string]interface{}
		if err := json.Unmarshal(content, &config); err != nil {
			continue
		}
		
		return config, nil
	}
	
	return nil, fmt.Errorf("no template configuration found")
}