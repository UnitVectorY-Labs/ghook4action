# Operations

## GitHub credentials

Create a fine-grained token owned by UnitVectorY-Labs, with organization **Projects: read and write**, repository **Issues: read**, and **Pull requests: read**, covering all public/private repositories that rules may match. Metadata read is implicit. The issuing user must be able to write both destination projects. Complete organization approval if required. No repository contents write or Actions permissions are needed.

Projects write is the documented project mutation permission; repository read access allows private issue/PR visibility. Validate both public and private routing before removing existing automation. See [GitHub project automation authentication](https://docs.github.com/en/issues/planning-and-tracking-with-projects/automating-your-project/using-the-api-to-manage-projects) and [fine-grained permissions](https://docs.github.com/en/rest/authentication/permissions-required-for-fine-grained-personal-access-tokens).

Store the token outside git, then provision the Kubernetes Secret through an approved deployment operation. Do not paste the token into an issue, pull request, chat, or shell command history. Mounted files are read only at startup. Use a finite expiry and rotate before expiration. A future GitHub App installation token can be supplied through the same interface, but automatic App token refresh is not implemented.

## Deployment and rollout

The Docker image uses nonroot distroless and supports a read-only root filesystem. Mount rules at `/config/rules.json` and token at `/secrets/github-token`. Use Application Default Credentials through the cluster's projected Workload Identity Federation token; no GCP service account key is needed. Grant `roles/pubsub.subscriber` only on the dedicated subscription.

The development workflow publishes `ghcr.io/unitvectory-labs/ghook4action-snapshot:dev` for AMD64 and ARM64. Prefer pinning its manifest digest for the first reviewed deployment. No live deployment should occur before the subscription IAM is applied and credentials are provisioned. Probe `/healthz` and `/readyz` on port 8080. Begin with one replica.

The existing `GitHub-PR-Issue-Opened` subscription is used by the UnitVectorY-Labs deployment. It can contain retained older events. Starting the consumer processes this backlog, which may add historical opened items to the configured projects. Inspect backlog age/count before deployment; do not purge or seek without approval. Review visibility changes since original publication when replaying old events: privacy matching reflects webhook-time state.

## Verification and rollback

First create a clearly labeled test issue on ghook4action after deployment and verify automatic inclusion in project 41 with the legacy workflow absent. A controlled pull request can test the other event path. Verify project 42 using an explicitly selected private repository. Close test artifacts afterwards and inspect logs for retry outcomes; do not claim private routing is verified based on a public test.

Scale the new deployment to zero to stop actions during rollback. Preserve the subscription and backlog. Keep existing workflows in other repositories until live public/private verification succeeds. Removing workflows across all repositories is a separate rollout and should not precede verification.

## Delivery failures

Watch subscriber backlog/oldest unacknowledged age and error logs. Authentication, missing project/repository access, rate limiting, malformed bodies, and unknown compression retain messages for retries. The initial infrastructure reuses an existing subscription with its existing retention policy and a 10–600 second retry backoff; a dead-letter policy is not provisioned. Repeated malformed messages need operational review before retention expires. Use the audit record to inspect failures securely. Fix permissions or config and restart, then verify the backlog drains. Do not silently acknowledge failed actions or clear the subscription.
