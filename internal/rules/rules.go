// Package rules validates and executes a declarative set of webhook actions.
package rules

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/UnitVectorY-Labs/ghook4action/internal/webhook"
)

type Config struct {
	Version int    `json:"version"`
	Rules   []Rule `json:"rules"`
}
type Rule struct {
	Name    string   `json:"name"`
	Match   Match    `json:"match"`
	Actions []Action `json:"actions"`
}
type Match struct {
	Events            []string `json:"events"`
	Actions           []string `json:"actions"`
	RepositoryOwner   string   `json:"repository_owner"`
	RepositoryPrivate *bool    `json:"repository_private"`
	Repositories      []string `json:"repositories,omitempty"`
}
type Action struct {
	Type    string  `json:"type"`
	Project Project `json:"project"`
}
type Project struct {
	Owner  string `json:"owner"`
	Number int    `json:"number"`
}
type ProjectAdder interface {
	AddToProject(context.Context, string, int, string) error
}

var ownerPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}$`)
var repoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}$`)

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("cannot read rules file")
	}
	return Parse(data)
}
func Parse(data []byte) (*Config, error) {
	if len(data) > 1<<20 {
		return nil, errors.New("rules file exceeds size limit")
	}
	var c Config
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return nil, errors.New("invalid rules JSON or unknown field")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, errors.New("rules must contain exactly one JSON object")
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}
func (c *Config) Validate() error {
	if c == nil || c.Version != 1 {
		return errors.New("rules version must be 1")
	}
	if len(c.Rules) == 0 {
		return errors.New("at least one rule is required")
	}
	names := map[string]bool{}
	for i, r := range c.Rules {
		fail := func(reason string) error { return fmt.Errorf("rule %d: %s", i+1, reason) }
		if strings.TrimSpace(r.Name) == "" || names[r.Name] {
			return fail("name must be nonempty and unique")
		}
		names[r.Name] = true
		if !ownerPattern.MatchString(r.Match.RepositoryOwner) {
			return fail("invalid repository owner")
		}
		if r.Match.RepositoryPrivate == nil {
			return fail("repository_private must be explicit")
		}
		if len(r.Match.Events) == 0 {
			return fail("events must be specified")
		}
		for _, e := range r.Match.Events {
			if e != "issues" && e != "pull_request" {
				return fail("unsupported event")
			}
		}
		if len(r.Match.Actions) == 0 {
			return fail("actions must be specified")
		}
		for _, a := range r.Match.Actions {
			if a != "opened" {
				return fail("unsupported webhook action")
			}
		}
		for _, repo := range r.Match.Repositories {
			if !repoPattern.MatchString(repo) {
				return fail("invalid repository name")
			}
		}
		if len(r.Actions) == 0 {
			return fail("at least one action is required")
		}
		for _, a := range r.Actions {
			if a.Type != "add_to_project" {
				return fail("unsupported action type")
			}
			if !ownerPattern.MatchString(a.Project.Owner) || a.Project.Number < 1 || a.Project.Number > 2147483647 {
				return fail("invalid project owner or number")
			}
		}
	}
	return nil
}
func contains(values []string, value string, fold bool) bool {
	for _, v := range values {
		if v == value || (fold && strings.EqualFold(v, value)) {
			return true
		}
	}
	return false
}

// Apply runs every matching rule. Successful earlier additions can safely repeat
// when a later action fails: GitHub returns the existing project item on retry.
func (c *Config) Apply(ctx context.Context, event *webhook.Event, adder ProjectAdder) (int, error) {
	if event == nil {
		return 0, errors.New("event is required")
	}
	completed := 0
	for _, r := range c.Rules {
		m := r.Match
		if !contains(m.Events, event.Type, false) || !contains(m.Actions, event.Action, false) || !strings.EqualFold(m.RepositoryOwner, event.RepositoryOwner) || m.RepositoryPrivate == nil || event.RepositoryPrivate == nil || *m.RepositoryPrivate != *event.RepositoryPrivate {
			continue
		}
		if len(m.Repositories) > 0 && !contains(m.Repositories, event.RepositoryName, true) {
			continue
		}
		if event.ContentID == "" {
			return completed, errors.New("matching event has no content node ID")
		}
		for _, a := range r.Actions {
			if err := adder.AddToProject(ctx, a.Project.Owner, a.Project.Number, event.ContentID); err != nil {
				return completed, fmt.Errorf("project action failed: %w", err)
			}
			completed++
		}
	}
	return completed, nil
}
