package rules

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/UnitVectorY-Labs/ghook4action/internal/webhook"
)

type dispatchFake struct {
	fakeAdder
	calls  int
	inputs map[string]string
}

func (f *dispatchFake) DispatchWorkflow(_ context.Context, owner, repo, id, ref string, inputs map[string]string) error {
	f.calls++
	f.inputs = inputs
	if owner != "example-org" || repo != "downstream" || id != "deploy.yml" || ref != "main" {
		panic("wrong destination")
	}
	return nil
}
func workflowConfig(t *testing.T) []byte {
	t.Helper()
	b, e := os.ReadFile("../../examples/workflows.json")
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestWorkflowRouting(t *testing.T) {
	cfg, err := Parse(workflowConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	private := true
	source := webhook.Event{Type: "workflow_run", Action: "completed", RepositoryOwner: "example-org", RepositoryName: "upstream", RepositoryPrivate: &private, Branch: "main", WorkflowPath: ".github/workflows/ci.yml", WorkflowEvent: "push", Conclusion: "success", SHA: strings.Repeat("a", 40), RunID: 123, RunAttempt: 2}
	for _, tt := range []struct {
		name   string
		change func(*webhook.Event)
	}{
		{"branch", func(e *webhook.Event) { e.Branch = "feature" }},
		{"workflow", func(e *webhook.Event) { e.WorkflowPath = ".github/workflows/other.yml" }},
		{"failure", func(e *webhook.Event) { e.Conclusion = "failure" }},
		{"cancelled", func(e *webhook.Event) { e.Conclusion = "cancelled" }},
		{"pr", func(e *webhook.Event) { e.WorkflowEvent = "pull_request" }},
		{"dispatch-loop", func(e *webhook.Event) { e.WorkflowEvent = "workflow_dispatch" }},
		{"repo", func(e *webhook.Event) { e.RepositoryName = "other" }},
		{"owner", func(e *webhook.Event) { e.RepositoryOwner = "other" }},
		{"privacy", func(e *webhook.Event) { p := false; e.RepositoryPrivate = &p }},
		{"action", func(e *webhook.Event) { e.Action = "in_progress" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := source
			tt.change(&e)
			f := &dispatchFake{}
			n, err := cfg.Apply(context.Background(), &e, f)
			if err != nil || n != 0 || f.calls != 0 {
				t.Fatalf("unmatched %d %v", n, err)
			}
		})
	}
	f := &dispatchFake{}
	n, err := cfg.Apply(context.Background(), &source, f)
	if err != nil || n != 1 || f.inputs["source_sha"] != source.SHA || f.inputs["source_run_id"] != "123" || f.inputs["source_run_attempt"] != "2" || f.inputs["environment"] != "staging" {
		t.Fatalf("dispatch %d %v %+v", n, err, f)
	}
	source.SHA = ""
	if _, err := cfg.Apply(context.Background(), &source, f); err == nil {
		t.Fatal("missing SHA accepted")
	}
}
func TestWorkflowValidation(t *testing.T) {
	b := string(workflowConfig(t))
	for _, tt := range []struct{ old, new string }{
		{`"branches": ["main"]`, `"branches": []`},
		{`"repositories": ["upstream"]`, `"repositories": []`},
		{`"conclusions": ["success"]`, `"conclusions": []`},
		{`"workflow_events": ["push"]`, `"workflow_events": ["pull_request"]`},
		{`"id": "deploy.yml"`, `"id": "../deploy.yml"`},
		{`"source_sha": "sha"`, `"source_sha": "arbitrary.expression"`},
		{`"workflow_paths": [".github/workflows/ci.yml"]`, `"workflow_paths": []`},
		{`"ref": "main"`, `"ref": ""`},
	} {
		if _, err := Parse([]byte(strings.Replace(b, tt.old, tt.new, 1))); err == nil {
			t.Fatalf("accepted %s", tt.new)
		}
	}
}
func TestPushRouting(t *testing.T) {
	cfg, err := Parse(workflowConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	m := &cfg.Rules[0].Match
	m.Events = []string{"push"}
	m.Actions = nil
	m.WorkflowPaths = nil
	m.WorkflowEvents = nil
	m.Conclusions = nil
	cfg.Rules[0].Actions[0].Workflow.SourceInputs = map[string]string{"source_sha": "sha"}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"refs/heads/main", "refs/heads/feature", "refs/tags/main"} {
		for _, deleted := range []bool{false, true} {
			payload := `{"ref":"` + ref + `","after":"` + strings.Repeat("a", 40) + `","deleted":` + map[bool]string{true: "true", false: "false"}[deleted] + `,"repository":{"name":"upstream","private":true,"owner":{"login":"example-org"}}}`
			e, err := webhook.Decode([]byte(payload), map[string]string{"gh_event": "push"})
			if err != nil {
				t.Fatal(err)
			}
			f := &dispatchFake{}
			n, err := cfg.Apply(context.Background(), e, f)
			want := 0
			if ref == "refs/heads/main" && !deleted {
				want = 1
			}
			if err != nil || n != want {
				t.Fatalf("%s deleted=%v: %d %v", ref, deleted, n, err)
			}
		}
	}
}
