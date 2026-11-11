// Package github provides scoped Projects v2 and workflow dispatch API actions.
package github

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const endpoint = "https://api.github.com/graphql"
const maxResponseBytes = 1 << 20

// Client never follows redirects and never exposes response bodies in errors.
type Client struct {
	token        string
	app          *appCredentials
	endpoint     string
	restEndpoint string
	http         *http.Client
	mu           sync.Mutex
	projects     map[string]string
}

func NewClient(token string) *Client {
	return &Client{token: token, endpoint: endpoint, restEndpoint: "https://api.github.com", http: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, projects: make(map[string]string)}
}

func (c *Client) request(ctx context.Context, query string, variables map[string]any, out any) error {
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
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return errors.New("cannot encode GitHub request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return errors.New("cannot create GitHub request")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ghook4action")
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New("GitHub request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized && c.app != nil {
			c.app.invalidate(token)
		}
		return fmt.Errorf("GitHub HTTP status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil || len(data) > maxResponseBytes {
		return errors.New("cannot read bounded GitHub response")
	}
	var envelope struct {
		Data   jsontext.Value   `json:"data"`
		Errors []jsontext.Value `json:"errors"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return errors.New("invalid GitHub response")
	}
	if len(envelope.Errors) > 0 {
		return errors.New("GitHub GraphQL operation failed")
	}
	if len(envelope.Data) == 0 || bytes.Equal(envelope.Data, []byte("null")) {
		return errors.New("missing GitHub response data")
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return errors.New("invalid GitHub operation data")
	}
	return nil
}
func validID(id string) bool {
	return id != "" && len(id) <= 512 && !strings.ContainsAny(id, " \t\r\n")
}
func (c *Client) projectID(ctx context.Context, owner string, number int) (string, error) {
	key := fmt.Sprintf("%s/%d", strings.ToLower(owner), number)
	c.mu.Lock()
	id := c.projects[key]
	c.mu.Unlock()
	if id != "" {
		return id, nil
	}
	var data struct {
		Organization *struct {
			Project *struct {
				ID string `json:"id"`
			} `json:"projectV2"`
		} `json:"organization"`
	}
	err := c.request(ctx, `query($owner:String!,$number:Int!){organization(login:$owner){projectV2(number:$number){id}}}`, map[string]any{"owner": owner, "number": number}, &data)
	if err != nil {
		return "", err
	}
	if data.Organization == nil || data.Organization.Project == nil || !validID(data.Organization.Project.ID) {
		return "", errors.New("project is unavailable or has invalid ID")
	}
	id = data.Organization.Project.ID
	c.mu.Lock()
	c.projects[key] = id
	c.mu.Unlock()
	return id, nil
}

// AddToProject is idempotent: adding existing content returns its existing item.
// See https://docs.github.com/en/issues/planning-and-tracking-with-projects/automating-your-project/using-the-api-to-manage-projects
func (c *Client) AddToProject(ctx context.Context, owner string, number int, contentID string) error {
	if owner == "" || number < 1 || !validID(contentID) {
		return errors.New("invalid project action arguments")
	}
	id, err := c.projectID(ctx, owner, number)
	if err != nil {
		return err
	}
	var data struct {
		Add *struct {
			Item *struct {
				ID string `json:"id"`
			} `json:"item"`
		} `json:"addProjectV2ItemById"`
	}
	err = c.request(ctx, `mutation($project:ID!,$content:ID!){addProjectV2ItemById(input:{projectId:$project,contentId:$content}){item{id}}}`, map[string]any{"project": id, "content": contentID}, &data)
	if err != nil {
		return err
	}
	if data.Add == nil || data.Add.Item == nil || !validID(data.Add.Item.ID) {
		return errors.New("GitHub did not confirm project item")
	}
	return nil
}

// CheckProject verifies that the configured project is accessible and caches its
// node ID. Callers should apply a startup deadline to ctx.
func (c *Client) CheckProject(ctx context.Context, owner string, number int) error {
	if owner == "" || number < 1 {
		return errors.New("invalid project check arguments")
	}
	_, err := c.projectID(ctx, owner, number)
	return err
}
