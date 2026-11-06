---
layout: default
title: ghook4action
nav_order: 1
permalink: /
---

# ghook4action

Route GitHub events to organization Projects and cross-repository workflow dispatches using explicit rules and Google Cloud Pub/Sub.

The application consumes verified messages published by ghook2pubsub. Each rule selects an organization, repository visibility, and optionally repository names, then adds matching items to one or more projects. Project routing supports opened issues and pull requests. Workflow orchestration supports branch pushes and completed workflow runs; see [workflow orchestration](WORKFLOWS.md).

## Documentation

- [Installation](INSTALL.md): prerequisites, credentials, and deployment.
- [Configuration](CONFIGURATION.md): environment variables and rule matching.
- [Usage](USAGE.md): validate rules, start the consumer, and verify routing.
- [Workflow orchestration](WORKFLOWS.md): upstream CI completion, downstream workflows, and push triggers.
- [Examples](EXAMPLES.md): public/private routing and repository restrictions.
- [Operations](OPERATIONS.md): credential rotation, retries, and rollback.
- [Architecture](ARCHITECTURE.md): message format and processing behavior.

## Related applications

- [ghook2pubsub](https://github.com/UnitVectorY-Labs/ghook2pubsub) receives GitHub webhooks, verifies signatures, and publishes events to Pub/Sub.
- [pubsub2postgresaudit](https://github.com/UnitVectorY-Labs/pubsub2postgresaudit) stores a separate subscription's messages in PostgreSQL for auditing.

Use separate subscriptions for the action consumer and audit consumer so both receive every relevant message.
