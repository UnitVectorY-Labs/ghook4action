---
layout: default
title: Usage
nav_order: 4
permalink: /usage
---

# Usage

## Prepare and validate rules

Copy [examples/rules.json](../examples/rules.json) to `rules.json`. Replace the organization and project numbers with your destinations. The shipped organization and project numbers are examples, not defaults. See [Examples](EXAMPLES.md) for a smaller starting point.

```sh
./ghook4action --rules-file rules.json --check-config
```

This validates rules only; it does not validate Google Cloud credentials, GitHub credentials, project permissions, or subscription connectivity.

## Start the consumer

With Google Cloud Application Default Credentials available and the App key stored outside the repository:

```sh
export PUBSUB_SUBSCRIPTION='projects/YOUR_PROJECT/subscriptions/ghook4action'
export RULES_FILE='./rules.json'
export GITHUB_APP_ID='YOUR_APP_ID'
export GITHUB_APP_INSTALLATION_ID='YOUR_INSTALLATION_ID'
export GITHUB_APP_PRIVATE_KEY_FILE='/secure/path/private-key.pem'
./ghook4action
```

For a container deployment, supply the same environment settings and mounts. Set `RULES_FILE=/config/rules.json` and `GITHUB_APP_PRIVATE_KEY_FILE=/secrets/private-key.pem`. The container requires ADC supplied by its runtime. Publish port 8080 if you need host access to probes.

Startup resolves every configured project before consuming messages. A project lookup confirms visibility; verify write access with a controlled event. Starting the consumer also processes retained backlog, so inspect backlog age and count first.

## Verify routing

```sh
curl --fail http://localhost:8080/healthz
curl --fail http://localhost:8080/readyz
```

`/healthz` returns `200` while the HTTP service is running. `/readyz` returns `503` until startup project checks complete, then `200`. Readiness does not continuously verify GitHub access or Pub/Sub connectivity.

Create a labeled test issue in a repository matching your rules. Confirm that it appears in the intended project, then close the test artifact. Repeat with a pull request and a selected private repository if those routes are configured.

Successful processing and valid unmatched events are acknowledged. Malformed messages and failed GitHub actions are retried; duplicate additions are safe because GitHub returns the existing project item. Monitor backlog and error logs as described in [Operations](OPERATIONS.md).

## Change rules or stop

Validate updated rules and restart the process to load them. Credential file changes also require a restart. Send SIGTERM or SIGINT for graceful shutdown; scale the deployment to zero to stop further actions while preserving backlog.

Use `./ghook4action --version` to inspect the build version. CLI flags `--rules-file`, `--pubsub-subscription`, and `--health-port` override their environment settings.
