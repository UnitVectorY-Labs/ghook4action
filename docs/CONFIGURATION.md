---
layout: default
title: Configuration
nav_order: 3
permalink: /configuration
---

# Configuration

Set `PUBSUB_SUBSCRIPTION` to the full `projects/PROJECT/subscriptions/SUBSCRIPTION` resource. Use a dedicated pull subscription; sharing a subscription with an audit consumer splits deliveries rather than broadcasting them.

Set `RULES_FILE` (default `rules.json`) to a mounted JSON rules file. Validate offline with `/server --rules-file /config/rules.json --check-config`. A restart is required to reload rules or rotated credential files.

Choose exactly one authentication mode. For GitHub App authentication (recommended), set all three:

- `GITHUB_APP_ID`: the numeric GitHub App ID.
- `GITHUB_APP_INSTALLATION_ID`: the positive installation ID for the destination organization.
- `GITHUB_APP_PRIVATE_KEY_FILE`: a mounted PEM private key file, such as `/secrets/private-key.pem`.

The client signs short-lived JWTs and requests installation tokens scoped to the configured action types: organization Projects write, Issues read, and Pull requests read for project actions; repository Actions write for workflow dispatches. Mixed configurations request both sets. It caches tokens in memory and renews before expiration. No client secret is needed. Tokens are never written to disk. Startup verifies visibility of every configured destination project and workflow before marking ready or consuming messages.

Alternatively supply `GITHUB_TOKEN_FILE` or `GITHUB_TOKEN`. These static token modes do not refresh tokens. App variables cannot be combined with token variables, and partial App configuration fails startup. Never put credentials in rules, command arguments, container images, or git.

`HEALTH_PORT` defaults to `8080`. CLI flags `--rules-file`, `--pubsub-subscription`, and `--health-port` override environment settings. `--version` prints the injected build version.

See [example rules](../examples/rules.json). The example routes public and private repositories to separate projects. Replace its organization and project numbers before use; these are examples, not application defaults.

Configuration version is `1`. Each named rule has `match` and `actions`. Match fields are ANDed; entries within `events` or `actions` are ORed. `repository_owner` and the explicit `repository_private` boolean are required. Optional `repositories` limits matching to those repository names. All matching rules execute in file order. The project action is `add_to_project`, whose project contains organization `owner` and positive project `number`.

Workflow rules add exact branch (or release-tag pattern), workflow path, origin event, and conclusion filters and the `workflow_dispatch` action. See [workflow orchestration](WORKFLOWS.md) and its [example configuration](../examples/workflows.json). Existing version 1 project configurations remain compatible.

Rules are data, with no scripts, shell interpolation, remote templates, or arbitrary expressions. Unknown keys and invalid configuration fail startup. Narrow organization matching prevents unrelated webhook traffic from invoking privileged actions.

## Settings reference

| Environment variable | Default | Purpose |
| --- | --- | --- |
| `PUBSUB_SUBSCRIPTION` | Required | Full dedicated pull subscription resource name. |
| `RULES_FILE` | `rules.json` | Local rules JSON file. |
| `HEALTH_PORT` | `8080` | HTTP liveness and readiness port. |
| `GITHUB_APP_ID` | Unset | GitHub App ID; use with both App settings below. |
| `GITHUB_APP_INSTALLATION_ID` | Unset | Organization installation ID. |
| `GITHUB_APP_PRIVATE_KEY_FILE` | Unset | Path to the App PEM private key. |
| `GITHUB_TOKEN_FILE` | Unset | Alternative static token file. |
| `GITHUB_TOKEN` | Unset | Alternative static token value. |

Choose App authentication or one static token source. See [Installation](INSTALL.md) for credentials and [Examples](EXAMPLES.md) for a complete rules file.
