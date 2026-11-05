package consumer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/UnitVectorY-Labs/ghook4action/internal/rules"
	"github.com/klauspost/compress/zstd"
)

type emulatorAdder struct {
	mu        sync.Mutex
	calls     map[string]int
	completed chan string
}

func (a *emulatorAdder) AddToProject(_ context.Context, owner string, number int, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if owner != "UnitVectorY-Labs" {
		return errors.New("wrong project owner")
	}
	a.calls[id]++
	switch id {
	case "I_public":
		if number != 41 {
			return errors.New("wrong public project")
		}
		if a.calls[id] == 1 {
			return errors.New("simulated transient action failure")
		}
	case "PR_private":
		if number != 42 {
			return errors.New("wrong private project")
		}
	default:
		return errors.New("unexpected content ID")
	}
	a.completed <- id
	return nil
}

// TestEmulatorDelivery exercises the actual Pub/Sub streaming subscriber without
// GCP credentials. Start a disposable Google Pub/Sub emulator and set its address
// in PUBSUB_EMULATOR_HOST. Never use a real GCP project for this test.
func TestEmulatorDelivery(t *testing.T) {
	if os.Getenv("PUBSUB_EMULATOR_HOST") == "" {
		t.Skip("requires a local Pub/Sub emulator")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client, err := pubsub.NewClient(ctx, "ghook4action-test")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	suffix := fmt.Sprint(time.Now().UnixNano())
	topic, err := client.CreateTopic(ctx, "test-"+suffix)
	if err != nil {
		t.Fatal(err)
	}
	defer topic.Stop()
	defer func() { _ = topic.Delete(context.Background()) }()
	sub, err := client.CreateSubscription(ctx, "test-"+suffix, pubsub.SubscriptionConfig{Topic: topic, AckDeadline: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sub.Delete(context.Background()) }()
	cfg, err := rules.Parse([]byte(`{"version":1,"rules":[{"name":"public","match":{"events":["issues","pull_request"],"actions":["opened"],"repository_owner":"UnitVectorY-Labs","repository_private":false},"actions":[{"type":"add_to_project","project":{"owner":"UnitVectorY-Labs","number":41}}]},{"name":"private","match":{"events":["issues","pull_request"],"actions":["opened"],"repository_owner":"UnitVectorY-Labs","repository_private":true},"actions":[{"type":"add_to_project","project":{"owner":"UnitVectorY-Labs","number":42}}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	adder := &emulatorAdder{calls: make(map[string]int), completed: make(chan string, 16)}
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- Run(runCtx, "projects/ghook4action-test/subscriptions/"+sub.ID(), cfg, adder) }()
	defer func() {
		stop()
		select {
		case err := <-done:
			if err != nil && ctx.Err() == nil {
				t.Errorf("subscriber: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("subscriber failed to stop")
		}
	}()
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()
	messages := []struct {
		kind        string
		data        []byte
		compression string
	}{
		{"issues", []byte(`{"action":"opened","repository":{"name":"test","private":false,"owner":{"login":"UnitVectorY-Labs"}},"issue":{"node_id":"I_public"}}`), ""},
		{"pull_request", encoder.EncodeAll([]byte(`{"action":"opened","repository":{"name":"test","private":true,"owner":{"login":"UnitVectorY-Labs"}},"pull_request":{"node_id":"PR_private"}}`), nil), "zstd"},
	}
	for _, m := range messages {
		if _, err := topic.Publish(ctx, &pubsub.Message{Data: m.data, Attributes: map[string]string{"gh_event": m.kind, "action": "opened", "compression": m.compression}}).Get(ctx); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case id := <-adder.completed:
			seen[id] = true
		case <-ctx.Done():
			t.Fatal("timed out waiting for routing and redelivery")
		}
	}
	// Allow the successful callback acknowledgements to reach the emulator, then
	// wait longer than the subscription deadline to catch unacknowledged repeats.
	timer := time.NewTimer(12 * time.Second)
	defer timer.Stop()
	select {
	case id := <-adder.completed:
		t.Fatalf("successful message redelivered: %s", id)
	case <-timer.C:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	adder.mu.Lock()
	defer adder.mu.Unlock()
	if adder.calls["I_public"] != 2 || adder.calls["PR_private"] != 1 {
		t.Fatalf("unexpected delivery counts: %v", adder.calls)
	}
}
