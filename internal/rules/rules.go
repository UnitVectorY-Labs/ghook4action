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
	"strconv"
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
	Branches          []string `json:"branches,omitempty"`
	WorkflowPaths     []string `json:"workflow_paths,omitempty"`
	WorkflowEvents    []string `json:"workflow_events,omitempty"`
	Conclusions       []string `json:"conclusions,omitempty"`
	Events            []string `json:"events"`
	Actions           []string `json:"actions"`
	RepositoryOwner   string   `json:"repository_owner"`
	RepositoryPrivate *bool    `json:"repository_private"`
	Repositories      []string `json:"repositories,omitempty"`
}
type Action struct {
	Type     string    `json:"type"`
	Project  Project   `json:"project,omitempty"`
	Workflow *Workflow `json:"workflow,omitempty"`
}
type Workflow struct {
	Owner        string            `json:"owner"`
	Repository   string            `json:"repository"`
	ID           string            `json:"id"`
	Ref          string            `json:"ref"`
	Inputs       map[string]string `json:"inputs,omitempty"`
	SourceInputs map[string]string `json:"source_inputs,omitempty"`
}
type WorkflowDispatcher interface {
	DispatchWorkflow(context.Context, string, string, string, string, map[string]string) error
}
type Project struct {
	Owner  string `json:"owner"`
	Number int    `json:"number"`
}
type ProjectAdder interface {
	AddToProject(context.Context, string, int, string) error
}

var shaPattern = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
var inputPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,99}$`)
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
		orchestration := contains(r.Match.Events, "workflow_run", false) || contains(r.Match.Events, "push", false)
		if orchestration {
			if len(r.Match.Events) != 1 || len(r.Match.Repositories) == 0 || len(r.Match.Branches) == 0 {
				return fail("workflow and push rules require one event, repositories and branches")
			}
			if r.Match.Events[0] == "workflow_run" {
				if len(r.Match.Actions) != 1 || r.Match.Actions[0] != "completed" || len(r.Match.WorkflowPaths) == 0 || len(r.Match.WorkflowEvents) == 0 || len(r.Match.Conclusions) == 0 {
					return fail("workflow_run requires completed action, workflow_paths, workflow_events and conclusions")
				}
				for _, e := range r.Match.WorkflowEvents {
					if e != "push" && e != "workflow_dispatch" && e != "schedule" && e != "repository_dispatch" && e != "workflow_run" {
						return fail("unsupported workflow origin event")
					}
				}
				for _, c := range r.Match.Conclusions {
					if !contains([]string{"success", "failure", "neutral", "cancelled", "skipped", "timed_out", "action_required", "stale", "startup_failure"}, c, false) {
						return fail("invalid workflow conclusion")
					}
				}
			} else if len(r.Match.Actions) != 0 || len(r.Match.WorkflowPaths)+len(r.Match.WorkflowEvents)+len(r.Match.Conclusions) != 0 {
				return fail("push rules have no actions or workflow filters")
			}
			for _, branch := range r.Match.Branches {
				if strings.TrimSpace(branch) == "" {
					return fail("empty branch")
				}
			}
			for _, path := range r.Match.WorkflowPaths {
				if !strings.HasPrefix(path, ".github/workflows/") || strings.Contains(path, "..") || strings.ContainsAny(path, "@?#") {
					return fail("invalid workflow path")
				}
			}
		} else {
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
			if len(r.Match.Branches)+len(r.Match.WorkflowPaths)+len(r.Match.WorkflowEvents)+len(r.Match.Conclusions) != 0 {
				return fail("workflow filters require workflow or push events")
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
			switch a.Type {
			case "add_to_project":
				if orchestration || a.Workflow != nil || !ownerPattern.MatchString(a.Project.Owner) || a.Project.Number < 1 || a.Project.Number > 2147483647 {
					return fail("invalid project action")
				}
			case "workflow_dispatch":
				w := a.Workflow
				if !orchestration || a.Project != (Project{}) || w == nil || !ownerPattern.MatchString(w.Owner) || !repoPattern.MatchString(w.Repository) || w.Repository == "." || w.Repository == ".." || !repoPattern.MatchString(w.ID) || w.ID == "." || w.ID == ".." || strings.TrimSpace(w.Ref) == "" || len(w.Ref) > 1024 || strings.ContainsAny(w.Ref, "\r\n") {
					return fail("invalid workflow destination")
				}
				if len(w.Inputs)+len(w.SourceInputs) > 25 {
					return fail("too many workflow inputs")
				}
				for key := range w.Inputs {
					if !inputPattern.MatchString(key) {
						return fail("invalid input name")
					}
				}
				for key, source := range w.SourceInputs {
					_, duplicate := w.Inputs[key]
					if !inputPattern.MatchString(key) || duplicate || !contains([]string{"repository", "branch", "sha", "event", "workflow_path", "run_id", "run_attempt"}, source, false) {
						return fail("invalid source input")
					}
					if r.Match.Events[0] == "push" && contains([]string{"workflow_path", "run_id", "run_attempt"}, source, false) {
						return fail("workflow source input requires workflow_run")
					}
				}
			default:
				return fail("unsupported action type")
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

// Apply runs every matching rule. Dispatches may repeat on redelivery or partial failure;
// downstream workflows must handle duplicate source revisions/runs.
func (c *Config) Apply(ctx context.Context, event *webhook.Event, adder ProjectAdder) (int, error) {
	if event == nil {
		return 0, errors.New("event is required")
	}
	completed := 0
	for _, r := range c.Rules {
		m := r.Match
		if !contains(m.Events, event.Type, false) || (event.Type != "push" && !contains(m.Actions, event.Action, false)) || !strings.EqualFold(m.RepositoryOwner, event.RepositoryOwner) || m.RepositoryPrivate == nil || event.RepositoryPrivate == nil || *m.RepositoryPrivate != *event.RepositoryPrivate {
			continue
		}
		if len(m.Repositories) > 0 && !contains(m.Repositories, event.RepositoryName, true) {
			continue
		}
		if len(m.Branches) > 0 && (!contains(m.Branches, event.Branch, false) || event.Deleted) {
			continue
		}
		if event.Type == "workflow_run" && (!contains(m.WorkflowPaths, event.WorkflowPath, false) || !contains(m.WorkflowEvents, event.WorkflowEvent, false) || !contains(m.Conclusions, event.Conclusion, false)) {
			continue
		}
		for _, a := range r.Actions {
			switch a.Type {
			case "add_to_project":
				if event.ContentID == "" {
					return completed, errors.New("matching event has no content node ID")
				}
				if err := adder.AddToProject(ctx, a.Project.Owner, a.Project.Number, event.ContentID); err != nil {
					return completed, fmt.Errorf("project action failed: %w", err)
				}
			case "workflow_dispatch":
				dispatcher, ok := adder.(WorkflowDispatcher)
				if !ok {
					return completed, errors.New("workflow dispatcher is unavailable")
				}
				if !shaPattern.MatchString(event.SHA) || (event.Type == "workflow_run" && (event.RunID <= 0 || event.RunAttempt <= 0)) {
					return completed, errors.New("matching event has invalid source revision or run")
				}
				w := a.Workflow
				inputs := make(map[string]string, len(w.Inputs)+len(w.SourceInputs))
				for k, v := range w.Inputs {
					inputs[k] = v
				}
				values := map[string]string{"repository": event.RepositoryOwner + "/" + event.RepositoryName, "branch": event.Branch, "sha": event.SHA, "event": event.Type, "workflow_path": event.WorkflowPath, "run_id": strconv.FormatInt(event.RunID, 10), "run_attempt": strconv.Itoa(event.RunAttempt)}
				for k, v := range w.SourceInputs {
					inputs[k] = values[v]
				}
				if err := dispatcher.DispatchWorkflow(ctx, w.Owner, w.Repository, w.ID, w.Ref, inputs); err != nil {
					return completed, fmt.Errorf("workflow action failed: %w", err)
				}
			}
			completed++
		}
	}
	return completed, nil
}
