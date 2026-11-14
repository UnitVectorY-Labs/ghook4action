package rules

import (
	"context"
	"encoding/json/v2"
	"strings"
	"testing"

	"github.com/UnitVectorY-Labs/ghook4action/internal/webhook"
)

func releaseConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := Parse(workflowConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	m := &cfg.Rules[0].Match
	m.Branches = nil
	m.TagPatterns = []string{"v[0-9]*", "release/*"}
	m.WorkflowEvents = []string{"release"}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	return cfg
}

// Exercise decoded tag refs, including the existing same-repository trust check,
// so a wildcard cannot make missing or fork head metadata eligible to dispatch.
func TestReleaseWorkflowRouting(t *testing.T) {
	cfg := releaseConfig(t)
	for _, tt := range []struct {
		name, ref, origin, conclusion, owner, repo, workflow string
		missingHead                                          bool
		want                                                 int
	}{
		{name: "version tag", ref: "v1.2.3", want: 1},
		{name: "other allowed pattern", ref: "release/1.2.3", want: 1},
		{name: "unmatched tag", ref: "nightly"},
		{name: "main branch", ref: "main"},
		{name: "empty ref"},
		{name: "failed release", ref: "v1.2.3", conclusion: "failure"},
		{name: "cancelled release", ref: "v1.2.3", conclusion: "cancelled"},
		{name: "PR on matching ref", ref: "v1.2.3", origin: "pull_request"},
		{name: "push on matching ref", ref: "v1.2.3", origin: "push"},
		{name: "dispatch loop", ref: "v1.2.3", origin: "workflow_dispatch"},
		{name: "fork owner", ref: "v1.2.3", owner: "fork-owner"},
		{name: "fork repository", ref: "v1.2.3", repo: "fork-repo"},
		{name: "missing head", ref: "v1.2.3", missingHead: true},
		{name: "other workflow", ref: "v1.2.3", workflow: ".github/workflows/other.yml"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.origin == "" {
				tt.origin = "release"
			}
			if tt.conclusion == "" {
				tt.conclusion = "success"
			}
			if tt.owner == "" {
				tt.owner = "example-org"
			}
			if tt.repo == "" {
				tt.repo = "upstream"
			}
			if tt.workflow == "" {
				tt.workflow = ".github/workflows/ci.yml"
			}
			var head any = map[string]any{"name": tt.repo, "owner": map[string]string{"login": tt.owner}}
			if tt.missingHead {
				head = nil
			}
			payload, err := json.Marshal(map[string]any{
				"action":       "completed",
				"repository":   map[string]any{"name": "upstream", "private": true, "owner": map[string]string{"login": "example-org"}},
				"workflow_run": map[string]any{"id": 123, "run_attempt": 1, "head_branch": tt.ref, "head_sha": strings.Repeat("a", 40), "event": tt.origin, "conclusion": tt.conclusion, "path": tt.workflow, "head_repository": head},
			})
			if err != nil {
				t.Fatal(err)
			}
			e, err := webhook.Decode(payload, map[string]string{"gh_event": "workflow_run", "action": "completed"})
			if err != nil {
				t.Fatal(err)
			}
			f := &dispatchFake{}
			n, err := cfg.Apply(context.Background(), e, f)
			if err != nil || n != tt.want || f.calls != tt.want {
				t.Fatalf("actions=%d calls=%d err=%v; want %d", n, f.calls, err, tt.want)
			}
			if tt.want > 0 && (f.inputs["source_sha"] != strings.Repeat("a", 40) || f.inputs["source_run_id"] != "123") {
				t.Fatalf("incorrect source mappings: %+v", f.inputs)
			}
		})
	}
	// Even an all-tag wildcard cannot match an untrusted or absent ref.
	cfg.Rules[0].Match.TagPatterns = []string{"*"}
	e := event()
	e.Type = "workflow_run"
	e.Action = "completed"
	e.WorkflowEvent = "release"
	e.WorkflowPath = ".github/workflows/ci.yml"
	e.Conclusion = "success"
	e.RepositoryOwner = "example-org"
	e.RepositoryName = "upstream"
	private := true
	e.RepositoryPrivate = &private
	if n, err := cfg.Apply(context.Background(), e, &dispatchFake{}); err != nil || n != 0 {
		t.Fatalf("empty ref matched wildcard: %d %v", n, err)
	}
}

func TestReleaseWorkflowValidation(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*Match)
	}{
		{"missing tag patterns", func(m *Match) { m.TagPatterns = nil }},
		{"empty pattern", func(m *Match) { m.TagPatterns = []string{""} }},
		{"blank pattern", func(m *Match) { m.TagPatterns = []string{"  "} }},
		{"malformed pattern", func(m *Match) { m.TagPatterns = []string{"v["} }},
		{"malformed wildcard suffix", func(m *Match) { m.TagPatterns = []string{"*["} }},
		{"newline pattern", func(m *Match) { m.TagPatterns = []string{"v*\n"} }},
		{"both branches and tags", func(m *Match) { m.Branches = []string{"main"} }},
		{"mixed origins", func(m *Match) { m.WorkflowEvents = []string{"release", "push"} }},
		{"tags on push-origin workflow", func(m *Match) { m.WorkflowEvents = []string{"push"}; m.Branches = []string{"main"} }},
		{"tags on push webhook", func(m *Match) {
			m.Events = []string{"push"}
			m.Actions = nil
			m.WorkflowPaths = nil
			m.WorkflowEvents = nil
			m.Conclusions = nil
			m.Branches = []string{"main"}
		}},
		{"tags on project rule", func(m *Match) {
			m.Events = []string{"issues"}
			m.Actions = []string{"opened"}
			m.WorkflowPaths = nil
			m.WorkflowEvents = nil
			m.Conclusions = nil
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := releaseConfig(t)
			tt.change(&cfg.Rules[0].Match)
			if err := cfg.Validate(); err == nil {
				t.Fatal("invalid release rule accepted")
			}
		})
	}
	// Round-trip the operator-facing JSON schema, not just the Go struct.
	cfg := releaseConfig(t)
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(b); err != nil {
		t.Fatal(err)
	}
}
