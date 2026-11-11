package github

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/UnitVectorY-Labs/ghook4action/internal/consumer"
	"github.com/UnitVectorY-Labs/ghook4action/internal/rules"
)

// This test uses a disposable Pub/Sub emulator and an HTTP mock, never a real
// subscription or GitHub destination. Use the emulator command in OPERATIONS.md.
func TestEmulatorWorkflowOrchestration(t *testing.T) {
	for _, origin := range []string{"push", "release"} {
		t.Run(origin, func(t *testing.T) { testEmulatorWorkflowOrchestration(t, origin) })
	}
}

func testEmulatorWorkflowOrchestration(t *testing.T, origin string) {
	if os.Getenv("PUBSUB_EMULATOR_HOST") == "" {
		t.Skip("requires isolated Pub/Sub emulator")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := pubsub.NewClient(ctx, "ghook4action-workflow-test")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	suffix := fmt.Sprint(time.Now().UnixNano())
	topic, err := client.CreateTopic(ctx, "workflow-"+suffix)
	if err != nil {
		t.Fatal(err)
	}
	defer topic.Stop()
	defer func() { _ = topic.Delete(context.Background()) }()
	sub, err := client.CreateSubscription(ctx, "workflow-"+suffix, pubsub.SubscriptionConfig{Topic: topic, AckDeadline: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sub.Delete(context.Background()) }()
	b, err := os.ReadFile("../../examples/workflows.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := rules.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	branch := "main"
	if origin == "release" {
		branch = "v1.2.3"
		cfg.Rules[0].Match.Branches = nil
		cfg.Rules[0].Match.TagPatterns = []string{"v[0-9]*"}
		cfg.Rules[0].Match.WorkflowEvents = []string{"release"}
		if err := cfg.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	var calls atomic.Int32
	completed := make(chan struct{}, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/example-org/downstream/actions/workflows/deploy.yml/dispatches" || r.Method != "POST" {
			t.Error("unexpected GitHub request")
			w.WriteHeader(404)
			return
		}
		var body struct {
			Ref    string            `json:"ref"`
			Inputs map[string]string `json:"inputs"`
		}
		if err := json.UnmarshalRead(r.Body, &body); err != nil || body.Ref != "main" || body.Inputs["source_sha"] != strings.Repeat("a", 40) || body.Inputs["source_run_id"] != "123" {
			t.Error("incorrect dispatch body")
		}
		if calls.Add(1) == 1 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(204)
		completed <- struct{}{}
	}))
	defer server.Close()
	gh := NewClient("test-only")
	gh.restEndpoint = server.URL
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- consumer.Run(runCtx, sub.String(), cfg, gh) }()
	defer func() {
		stop()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(10 * time.Second):
			t.Error("subscriber did not stop")
		}
	}()
	for _, conclusion := range []string{"failure", "success"} {
		payload := `{"action":"completed","repository":{"name":"upstream","private":true,"owner":{"login":"example-org"}},"workflow_run":{"id":123,"run_attempt":1,"head_branch":"` + branch + `","head_sha":"` + strings.Repeat("a", 40) + `","path":".github/workflows/ci.yml","event":"` + origin + `","conclusion":"` + conclusion + `","head_repository":{"name":"upstream","owner":{"login":"example-org"}}}}`
		if _, err := topic.Publish(ctx, &pubsub.Message{Data: []byte(payload), Attributes: map[string]string{"gh_event": "workflow_run", "action": "completed"}}).Get(ctx); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-completed:
	case <-ctx.Done():
		t.Fatal("dispatch/redelivery timed out")
	}
	select {
	case <-completed:
		t.Fatal("acknowledged dispatch repeated")
	case <-time.After(12 * time.Second):
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if calls.Load() != 2 {
		t.Fatalf("expected failed request and successful retry, got %d", calls.Load())
	}
}
