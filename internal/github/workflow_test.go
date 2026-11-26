package github

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkflowAPI(t *testing.T) {
	for _, status := range []int{200, 204, 401, 403, 404, 422, 429, 500, 302} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/repos/org/repo/actions/workflows/deploy.yml/dispatches" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer secret" {
					t.Error("invalid request")
				}
				var body struct {
					Ref    string            `json:"ref"`
					Inputs map[string]string `json:"inputs"`
				}
				if err := json.UnmarshalRead(r.Body, &body); err != nil || body.Ref != "main" || body.Inputs["sha"] != "abc" {
					t.Error("invalid body")
				}
				w.Header().Set("Location", "https://example.invalid/never-follow")
				w.WriteHeader(status)
				w.Write([]byte("sensitive response"))
			}))
			defer server.Close()
			c := NewClient("secret")
			c.restEndpoint = server.URL
			err := c.DispatchWorkflow(context.Background(), "org", "repo", "deploy.yml", "main", map[string]string{"sha": "abc"})
			if (err == nil) != (status == 200 || status == 204) {
				t.Fatalf("status %d: %v", status, err)
			}
			if err != nil && strings.Contains(err.Error(), "sensitive") {
				t.Fatal("response leaked")
			}
		})
	}
}
func TestWorkflowPermissionScope(t *testing.T) {
	c := appClient(t, testKey(t))
	c.ConfigurePermissions(false, true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Permissions map[string]string `json:"permissions"`
		}
		if err := json.UnmarshalRead(r.Body, &body); err != nil {
			t.Error(err)
		}
		if len(body.Permissions) != 1 || body.Permissions["actions"] != "write" {
			t.Errorf("permissions: %v", body.Permissions)
		}
		w.WriteHeader(403)
	}))
	defer server.Close()
	c.app.endpoint = server.URL
	c.app.token(context.Background(), c.http)
	c.ConfigurePermissions(true, true)
	if len(c.app.permissions) != 4 {
		t.Fatal("mixed permissions missing")
	}
}

func TestWorkflowPreflight(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/repos/org/repo/actions/workflows/deploy.yml" || r.ContentLength != 0 {
			t.Error("preflight should only read workflow metadata")
		}
		w.Write([]byte(`{"id":123,"state":"active"}`))
	}))
	defer server.Close()
	c := NewClient("secret")
	c.restEndpoint = server.URL
	if err := c.CheckWorkflow(context.Background(), "org", "repo", "deploy.yml"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"../deploy.yml", "https://example.com/file", "..", ""} {
		if err := c.CheckWorkflow(context.Background(), "org", "repo", id); err == nil {
			t.Fatalf("invalid workflow %q accepted", id)
		}
	}
}
