package github

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAddIdempotentAndCachesProject(t *testing.T) {
	queries, mutations := 0, 0
	items := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing auth")
		}
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.UnmarshalRead(r.Body, &req); err != nil {
			t.Error(err)
			return
		}
		if strings.HasPrefix(req.Query, "query") {
			queries++
			fmt.Fprint(w, `{"data":{"organization":{"projectV2":{"id":"PVT_1"}}}}`)
			return
		}
		mutations++
		id := req.Variables["content"].(string)
		if _, ok := items[id]; !ok {
			items[id] = "PVTI_1"
		}
		fmt.Fprintf(w, `{"data":{"addProjectV2ItemById":{"item":{"id":%q}}}}`, items[id])
	}))
	defer server.Close()
	c := NewClient("test-token")
	c.endpoint = server.URL
	for range 2 {
		if err := c.AddToProject(context.Background(), "Org", 41, "I_1"); err != nil {
			t.Fatal(err)
		}
	}
	if queries != 1 || mutations != 2 || len(items) != 1 {
		t.Fatalf("queries %d mutations %d items %d", queries, mutations, len(items))
	}
}
func TestRejectErrorsWithoutLeakingBody(t *testing.T) {
	for _, body := range []string{`{"errors":[{"message":"private secret details"}]}`, `{"data":{"organization":null}}`, `{"data":null}`, `not JSON`, `{"data":{},"data":{"organization":{"projectV2":{"id":"PVT_1"}}}}`, `{"data":{"organization":{"projectV2":{"id":"PVT_1","id":"PVT_2"}}}}`, strings.Repeat("x", maxResponseBytes+1)} {
		t.Run(fmt.Sprint(len(body)), func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer s.Close()
			c := NewClient("test")
			c.endpoint = s.URL
			err := c.AddToProject(context.Background(), "Org", 41, "I_1")
			if err == nil || strings.Contains(err.Error(), "private secret details") {
				t.Fatalf("unsafe error: %v", err)
			}
		})
	}
}
func TestNoRedirectAuthLeak(t *testing.T) {
	called := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	c := NewClient("secret")
	c.endpoint = source.URL
	if err := c.AddToProject(context.Background(), "Org", 1, "I_1"); err == nil || called {
		t.Fatal("redirect followed")
	}
}
func TestMissingMutationConfirmation(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"addProjectV2ItemById":{"item":null}}}`)
	}))
	defer s.Close()
	c := NewClient("test")
	c.endpoint = s.URL
	c.projects["org/1"] = "PVT_1"
	if err := c.AddToProject(context.Background(), "Org", 1, "I_1"); err == nil {
		t.Fatal("missing item accepted")
	}
}
