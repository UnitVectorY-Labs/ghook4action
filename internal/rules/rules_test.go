package rules

import (
	"context"
	"errors"
	"github.com/UnitVectorY-Labs/ghook4action/internal/webhook"
	"strings"
	"testing"
)

const configJSON = `{"version":1,"rules":[{"name":"public","match":{"events":["issues","pull_request"],"actions":["opened"],"repository_owner":"Org","repository_private":false},"actions":[{"type":"add_to_project","project":{"owner":"Org","number":41}}]}]}`

type fakeAdder struct {
	calls []int
	fail  int
	items map[int]bool
}

func (f *fakeAdder) AddToProject(_ context.Context, _ string, n int, _ string) error {
	f.calls = append(f.calls, n)
	if n == f.fail {
		return errors.New("temporary API failure")
	}
	if f.items == nil {
		f.items = map[int]bool{}
	}
	f.items[n] = true
	return nil
}
func event() *webhook.Event {
	p := false
	return &webhook.Event{Type: "issues", Action: "opened", RepositoryOwner: "org", RepositoryName: "repo", RepositoryPrivate: &p, ContentID: "I_1"}
}
func TestRulesMatchPrivacyAndRepository(t *testing.T) {
	c, err := Parse([]byte(configJSON))
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeAdder{}
	e := event()
	if n, err := c.Apply(context.Background(), e, f); err != nil || n != 1 {
		t.Fatalf("apply %d %v", n, err)
	}
	p := true
	e.RepositoryPrivate = &p
	if n, _ := c.Apply(context.Background(), e, f); n != 0 {
		t.Fatal("private matched public")
	}
	e.RepositoryPrivate = nil
	if n, _ := c.Apply(context.Background(), e, f); n != 0 {
		t.Fatal("missing privacy matched")
	}
	e = event()
	c.Rules[0].Match.Repositories = []string{"another"}
	if n, _ := c.Apply(context.Background(), e, f); n != 0 {
		t.Fatal("wrong repository matched")
	}
}
func TestStrictValidation(t *testing.T) {
	for _, bad := range []string{strings.Replace(configJSON, `"version":1`, `"version":2`, 1), strings.Replace(configJSON, `"repository_private":false`, `"unknown":false`, 1), strings.Replace(configJSON, `"opened"`, `"closed"`, 1), strings.Replace(configJSON, `"add_to_project"`, `"execute"`, 1), configJSON + ` {}`} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Fatal("invalid rules accepted")
		}
	}
}
func TestPartialActionsRetrySafely(t *testing.T) {
	c, _ := Parse([]byte(configJSON))
	r := c.Rules[0]
	r.Name = "second"
	r.Actions = []Action{{Type: "add_to_project", Project: Project{Owner: "Org", Number: 42}}}
	c.Rules = append(c.Rules, r)
	f := &fakeAdder{fail: 42}
	if n, err := c.Apply(context.Background(), event(), f); err == nil || n != 1 {
		t.Fatalf("partial result %d %v", n, err)
	}
	f.fail = 0
	if n, err := c.Apply(context.Background(), event(), f); err != nil || n != 2 {
		t.Fatalf("retry %d %v", n, err)
	}
	if len(f.items) != 2 || len(f.calls) != 4 {
		t.Fatalf("unexpected effects %+v", f)
	}
}
