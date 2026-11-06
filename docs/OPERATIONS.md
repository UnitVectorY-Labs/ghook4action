---
layout: default
title: Operations
nav_order: 6
permalink: /operations
---

# Operations

## GitHub credentials

Register a private GitHub App owned by the organization and install it on all repositories that rules may match. Grant organization **Projects: read and write**, repository **Issues: read**, and **Pull requests: read**. Metadata read is implicit. Webhooks can remain disabled because ghook2pubsub already receives events. Project-only rules need no client secret, repository contents permission, or Actions permission. Workflow dispatch rules require repository **Actions: read and write** on destination repositories; dispatch-only rules need none of the Projects/Issues/Pull requests permissions. See [Workflow orchestration](WORKFLOWS.md) for source webhook permissions and subscription filters.

Generate the App's RSA private key and store the PEM outside git, accessible only to the operating account. Mount it read-only using your runtime’s secret storage. Configure the App ID, installation ID, and key path as described in [configuration](CONFIGURATION.md). The service generates installation access tokens and automatically renews them before their one-hour expiration. Concurrent callbacks share the cached token and serialized refresh. Requests have bounded timeouts and refuse redirects. Failure responses never expose token or body contents in logs. See [GitHub App authentication](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/authenticating-as-a-github-app-installation).

Startup checks that every configured project or workflow can be resolved before consuming messages. This validates project visibility, but a controlled live test is still required to verify write access and private repository access. For a private repository, do not infer access from public project lookup alone.

Rotate the private key by generating a new one, updating the mounted secret, and restarting the deployment. Verify successful token generation and message processing before revoking the old key. The process reads key files at startup. Do not paste keys or tokens into chat, issues, shell history, or source control.

Static fine-grained tokens remain supported as an alternative. Grant the same organization and repository permissions, cover the required repositories, and mount through `GITHUB_TOKEN_FILE`. Static tokens require operator-managed rotation and expiry monitoring.

## Deployment and rollout

The Docker image uses nonroot distroless and supports a read-only root filesystem. Mount rules at `/config/rules.json` and the App private key at `/secrets/private-key.pem`. Supply Google Cloud Application Default Credentials through your runtime, preferably using an attached workload identity. Grant `roles/pubsub.subscriber` only on the dedicated subscription.

The development snapshot image is mutable. For controlled upgrades, deploy a reviewed image digest and verify the application version after rollout. Provision subscription access and GitHub credentials before starting the consumer. Probe `/healthz` and `/readyz` on port 8080. Start with one replica and monitor processing before increasing capacity.

A subscription can contain retained older events. Starting the consumer processes this backlog, which may add historical opened items to the configured projects. Inspect backlog age/count before deployment; do not purge or seek without approval. Review visibility changes since original publication when replaying old events: privacy matching reflects webhook-time state.

## Verification and rollback

First create a clearly labeled test issue in a matching repository and verify automatic inclusion in the configured project. A controlled pull request can test the other event path. Verify private routing using an explicitly selected private repository. Close test artifacts afterwards and inspect logs for retry outcomes; do not claim private routing is verified based on a public test.

For workflow rules, complete a controlled run in an explicitly selected upstream repository and verify that the expected downstream workflow receives its source inputs. Confirm that a failed run and a run on another branch do not dispatch. Preflight visibility checks do not verify write access, and a successful dispatch does not establish downstream job success.

Scale the new deployment to zero to stop actions during rollback. Preserve the subscription and backlog. If replacing an existing automation, verify the new routes before retiring it.

## Delivery failures

Watch subscriber backlog/oldest unacknowledged age and error logs. Authentication, missing project/repository access, rate limiting, malformed bodies, and unknown compression retain messages for retries. Configure retry backoff and a dead-letter policy on your subscription; the application does not provision them. Repeated malformed messages need operational review before retention expires. Use the audit record to inspect failures securely. Fix permissions or config and restart, then verify the backlog drains. Do not silently acknowledge failed actions or clear the subscription.

## Local integration tests

Run subscriber tests only against a disposable emulator, never a production subscription. Bind it to loopback and publish only synthetic messages. For example:

```sh
docker run --rm -d --name ghook4action-test-emulator \
  -p 127.0.0.1:8085:8085 \
  gcr.io/google.com/cloudsdktool/google-cloud-cli:emulators \
  gcloud beta emulators pubsub start --project=ghook4action-test --host-port=0.0.0.0:8085
PUBSUB_EMULATOR_HOST=127.0.0.1:8085 go test -race -count=1 ./...
docker stop ghook4action-test-emulator
go vet ./...
```

Wait for emulator startup before running tests. Tests create unique topics and subscriptions in test projects. The workflow integration test uses a mock GitHub HTTP server and checks dispatch payloads, failure redelivery, nonmatching completions, and acknowledgements. Without `PUBSUB_EMULATOR_HOST`, integration tests skip; ordinary unit tests still run.
