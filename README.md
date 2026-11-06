[![License](https://img.shields.io/badge/license-MIT-blue.svg)](https://opensource.org/licenses/MIT) [![Work In Progress](https://img.shields.io/badge/Status-Work%20In%20Progress-yellow)](https://guide.unitvectorylabs.com/bestpractices/status/#work-in-progress) 

# ghook4action

A configurable rules engine for GitHub webhook events delivered through Google Cloud Pub/Sub.

Consumes verified webhook messages from ghook2pubsub, evaluates explicit repository rules, adds matching issues and pull requests to GitHub organization projects, and triggers downstream workflows after configured branch pushes or workflow completions. No privileged pull request workflow is required.

## Documentation

See the [documentation](docs/README.md) for [installation](docs/INSTALL.md), [configuration](docs/CONFIGURATION.md), [usage](docs/USAGE.md), and [examples](docs/EXAMPLES.md).

## Related applications

- [ghook2pubsub](https://github.com/UnitVectorY-Labs/ghook2pubsub) receives GitHub webhooks, verifies signatures, and publishes events to Pub/Sub for this application to consume.
- [pubsub2postgresaudit](https://github.com/UnitVectorY-Labs/pubsub2postgresaudit) stores webhook messages in PostgreSQL for auditing through its own subscription, independently of this application's project actions.
