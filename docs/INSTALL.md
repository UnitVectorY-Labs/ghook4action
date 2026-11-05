---
layout: default
title: Installation
nav_order: 2
permalink: /install
---

# Installation

## Prerequisites

- A Pub/Sub topic receiving verified GitHub webhook events from [ghook2pubsub](https://github.com/UnitVectorY-Labs/ghook2pubsub).
- A dedicated pull subscription on that topic, with `roles/pubsub.subscriber` granted to the application's Google Cloud identity. Do not share the audit consumer's subscription: consumers on one subscription divide messages.
- GitHub organization Projects and credentials that can read the source issues/pull requests and write to destination projects.
- Docker, or the Go version required by [go.mod](../go.mod) to build from source.

A useful subscription filter is:

```text
attributes.action = "opened" AND (attributes.gh_event = "issues" OR attributes.gh_event = "pull_request")
```

Repository visibility is in the webhook body and cannot be filtered through these attributes.

## Configure GitHub access

Create and install a GitHub App on all repositories your rules may match. Grant organization **Projects: read and write**, repository **Issues: read**, and **Pull requests: read**. App webhooks may remain disabled. Generate a private key, store it outside git, and mount it read-only. Set the App ID, installation ID, and key path described in [Configuration](CONFIGURATION.md). Installation tokens renew automatically.

Alternatively mount a static token through `GITHUB_TOKEN_FILE` with equivalent access. You must manage its expiration and rotation.

## Build from source

```sh
git clone https://github.com/UnitVectorY-Labs/ghook4action.git
cd ghook4action
go build -o ghook4action .
./ghook4action --rules-file examples/rules.json --check-config
```

The check prints `rules configuration valid` without network access. Copy and customize the rules before starting a consumer.

## Container image

The development snapshot is `ghcr.io/unitvectory-labs/ghook4action-snapshot:dev` for AMD64 and ARM64. The tag is mutable; pin a reviewed digest for a controlled deployment. You can also build locally with `docker build -t ghook4action .`.

The image runs `/server` as UID/GID 65532. Ensure mounted rules and secret files are readable by this user. Mount rules at `/config/rules.json`, the App key at `/secrets/private-key.pem`, and configure Application Default Credentials for Google Cloud. On supported cloud deployments, use an attached workload identity. For local execution, use your configured ADC credentials.

Continue with [Usage](USAGE.md).
