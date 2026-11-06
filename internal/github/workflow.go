package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// ConfigurePermissions selects installation token permissions at startup, before
// the client is used. Dispatch-only deployments do not request Projects access.
func (c *Client) ConfigurePermissions(projects, workflows bool) {
	if c.app == nil {
		return
	}
	permissions := map[string]string{}
	if projects {
		permissions["organization_projects"] = "write"
		permissions["issues"] = "read"
		permissions["pull_requests"] = "read"
	}
	if workflows {
		permissions["actions"] = "write"
	}
	c.app.permissions = permissions
}

var pathSegment = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}$`)

func workflowPath(owner, repo, workflow string) (string, error) {
	for _, part := range []string{owner, repo, workflow} {
		if !pathSegment.MatchString(part) || part == "." || part == ".." {
			return "", errors.New("invalid workflow destination")
		}
	}
	return "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/actions/workflows/" + url.PathEscape(workflow), nil
}

func (c *Client) workflowRequest(ctx context.Context, method, path string, body any) error {
	token := c.token
	if c.app != nil {
		var err error
		token, err = c.app.token(ctx, c.http)
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(token) == "" {
		return errors.New("GitHub token is required")
	}
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			return errors.New("cannot encode workflow request")
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.restEndpoint+path, bytes.NewReader(data))
	if err != nil {
		return errors.New("cannot create workflow request")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	req.Header.Set("User-Agent", "ghook4action")
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New("GitHub workflow request failed")
	}
	defer resp.Body.Close()
	// The current API returns 200 with run metadata; older versions return 204.
	if resp.StatusCode == http.StatusOK || (method == http.MethodPost && resp.StatusCode == http.StatusNoContent) {
		return nil
	}
	if resp.StatusCode == http.StatusUnauthorized && c.app != nil {
		c.app.invalidate(token)
	}
	return fmt.Errorf("GitHub workflow HTTP status %d", resp.StatusCode)
}

// CheckWorkflow checks destination visibility without dispatching it. This cannot
// prove write permission or validate the workflow's input schema or ref.
func (c *Client) CheckWorkflow(ctx context.Context, owner, repo, workflow string) error {
	path, err := workflowPath(owner, repo, workflow)
	if err != nil {
		return err
	}
	return c.workflowRequest(ctx, http.MethodGet, path, nil)
}

// DispatchWorkflow uses only operator-configured destinations, never payload URLs.
func (c *Client) DispatchWorkflow(ctx context.Context, owner, repo, workflow, ref string, inputs map[string]string) error {
	path, err := workflowPath(owner, repo, workflow)
	if err != nil {
		return err
	}
	if strings.TrimSpace(ref) == "" {
		return errors.New("workflow ref is required")
	}
	return c.workflowRequest(ctx, http.MethodPost, path+"/dispatches", map[string]any{"ref": ref, "inputs": inputs})
}
