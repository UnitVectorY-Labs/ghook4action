// Package consumer connects the rules engine to a bounded Pub/Sub subscriber.
package consumer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/UnitVectorY-Labs/ghook4action/internal/rules"
	"github.com/UnitVectorY-Labs/ghook4action/internal/webhook"
)

func ParseSubscription(name string) (string, string, error) {
	p := strings.Split(name, "/")
	if len(p) != 4 || p[0] != "projects" || p[1] == "" || p[2] != "subscriptions" || p[3] == "" {
		return "", "", fmt.Errorf("subscription must be projects/PROJECT/subscriptions/NAME")
	}
	return p[1], p[3], nil
}

// Process acknowledges only a successful evaluation, including intentionally unmatched events.
// Decode and action failures remain unacknowledged for recovery rather than being discarded.
func Process(ctx context.Context, cfg *rules.Config, adder rules.ProjectAdder, data []byte, attrs map[string]string) (int, error) {
	event, err := webhook.Decode(data, attrs)
	if err != nil {
		return 0, fmt.Errorf("decode: %w", err)
	}
	return cfg.Apply(ctx, event, adder)
}

func Run(ctx context.Context, subscription string, cfg *rules.Config, adder rules.ProjectAdder) error {
	project, subID, err := ParseSubscription(subscription)
	if err != nil {
		return err
	}
	client, err := pubsub.NewClient(ctx, project)
	if err != nil {
		return fmt.Errorf("create subscriber: %w", err)
	}
	defer client.Close()
	sub := client.Subscription(subID)
	sub.ReceiveSettings.MaxOutstandingMessages = 4
	sub.ReceiveSettings.MaxOutstandingBytes = 32 * 1024 * 1024
	sub.ReceiveSettings.NumGoroutines = 1
	return sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		started := time.Now()
		applied, err := Process(ctx, cfg, adder, msg.Data, msg.Attributes)
		if err != nil {
			// Decode and GitHub errors deliberately redact payloads and response bodies.
			slog.Error("message processing failed; will retry", "message_id", msg.ID, "outcome", "retry", "reason", err.Error(), "elapsed_ms", time.Since(started).Milliseconds())
			msg.Nack()
			return
		}
		slog.Info("message processed", "message_id", msg.ID, "actions", applied, "outcome", "ack", "elapsed_ms", time.Since(started).Milliseconds())
		msg.Ack()
	})
}
