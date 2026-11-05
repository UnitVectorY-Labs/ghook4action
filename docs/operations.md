# Operations

## GitHub credentials

Register a private GitHub App owned by the organization and install it on all repositories that rules may match. Grant organization **Projects: read and write**, repository **Issues: read**, and **Pull requests: read**. Metadata read is implicit. Webhooks can remain disabled because ghook2pubsub already receives events. No client secret, repository contents permission, or Actions permission is needed.

Generate the App's RSA private key and store the PEM outside git, accessible only to the operating account. Mount it read-only from a Kubernetes Secret. Configure the App ID, installation ID, and key path as described in [configuration](configuration.md). The service generates installation access tokens and automatically renews them before their one-hour expiration. Concurrent callbacks share the cached token and serialized refresh. Requests have bounded timeouts and refuse redirects. Failure responses never expose token or body contents in logs. See [GitHub App authentication](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/authenticating-as-a-github-app-installation).

Startup checks that every configured project can be resolved before consuming messages. This validates project visibility, but a controlled live test is still required to verify write access and private repository access. For a private repository, do not infer access from public project lookup alone.

Rotate the private key by generating a new one, updating the Secret, and restarting the deployment. Verify successful token generation and message processing before revoking the old key. The process reads key files at startup. Do not paste keys or tokens into chat, issues, shell history, or source control.

Static fine-grained tokens remain supported as an alternative. Grant the same organization and repository permissions, cover the required repositories, and mount through `GITHUB_TOKEN_FILE`. Static tokens require operator-managed rotation and expiry monitoring.

## Deployment and rollout

The Docker image uses nonroot distroless and supports a read-only root filesystem. Mount rules at `/config/rules.json` and the App private key at `/secrets/private-key.pem`. Use Application Default Credentials through the cluster's projected Workload Identity Federation token; no GCP service account key is needed. Grant `roles/pubsub.subscriber` only on the dedicated subscription.

The development workflow publishes `ghcr.io/unitvectory-labs/ghook4action-snapshot:dev` for AMD64 and ARM64. Prefer pinning its manifest digest for the first reviewed deployment. No live deployment should occur before the subscription IAM is applied and credentials are provisioned. Probe `/healthz` and `/readyz` on port 8080. Begin with one replica.

The existing `GitHub-PR-Issue-Opened` subscription is used by the UnitVectorY-Labs deployment. It can contain retained older events. Starting the consumer processes this backlog, which may add historical opened items to the configured projects. Inspect backlog age/count before deployment; do not purge or seek without approval. Review visibility changes since original publication when replaying old events: privacy matching reflects webhook-time state.

## Verification and rollback

First create a clearly labeled test issue on ghook4action after deployment and verify automatic inclusion in project 41 with the legacy workflow absent. A controlled pull request can test the other event path. Verify project 42 using an explicitly selected private repository. Close test artifacts afterwards and inspect logs for retry outcomes; do not claim private routing is verified based on a public test.

Scale the new deployment to zero to stop actions during rollback. Preserve the subscription and backlog. Keep existing workflows in other repositories until live public/private verification succeeds. Removing workflows across all repositories is a separate rollout and should not precede verification.

## Delivery failures

Watch subscriber backlog/oldest unacknowledged age and error logs. Authentication, missing project/repository access, rate limiting, malformed bodies, and unknown compression retain messages for retries. The initial infrastructure reuses an existing subscription with its existing retention policy and a 10–600 second retry backoff; a dead-letter policy is not provisioned. Repeated malformed messages need operational review before retention expires. Use the audit record to inspect failures securely. Fix permissions or config and restart, then verify the backlog drains. Do not silently acknowledge failed actions or clear the subscription.
