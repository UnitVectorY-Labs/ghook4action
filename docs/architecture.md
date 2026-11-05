# Architecture

GitHub → ghook2pubsub → Pub/Sub topic → dedicated pull subscription → ghook4action → GitHub Projects API.

The application consumes the original webhook body, not a Pub/Sub push envelope. It supports JSON and form-urlencoded `payload` bodies, plus gzip/zstd compression marked by the `compression` message attribute. Decoded data is bounded to limit memory use. `gh_event` identifies the webhook kind; the payload supplies action, base repository privacy/owner/name, and the issue or pull request node ID. Pull request fork privacy is never used for routing.

The subscription should filter on `attributes.action = "opened" AND (attributes.gh_event = "issues" OR attributes.gh_event = "pull_request")`. Filtering reduces traffic; rule matching still runs in the application. Privacy cannot be filtered at subscription level because it is inside the body.

The GitHub client resolves configured organization/project numbers and calls `addProjectV2ItemById`. GitHub returns the existing item when it is already present, so duplicate deliveries and retries after partial rule execution are safe. See [GitHub Projects API documentation](https://docs.github.com/en/issues/planning-and-tracking-with-projects/automating-your-project/using-the-api-to-manage-projects).

Only successful processing and valid unmatched events are acknowledged. Decode failures and GitHub/network/permission errors are negatively acknowledged. No payload is logged. Failure logs contain a redacted failure reason and the Pub/Sub message ID for correlation with the separate audit stream. Configure subscription backoff/dead lettering operationally to avoid repeated poison deliveries; without a dead-letter policy failed messages retry until retention expires. A failed action may have succeeded remotely; retries intentionally repeat idempotent actions.

Subscriber flow control limits concurrent outstanding messages. Shutdown cancels Receive and lets in-flight callbacks finish. Readiness remains false until startup resolves all configured projects using the selected credentials. It does not continuously check downstream GitHub permission or Pub/Sub connectivity. GitHub App installation tokens are refreshed in memory before expiration. No inbound webhook endpoint, repository checkout, or code execution is needed.
