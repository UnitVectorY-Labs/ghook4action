[![License](https://img.shields.io/badge/license-MIT-blue.svg)](https://opensource.org/licenses/MIT) [![Work In Progress](https://img.shields.io/badge/Status-Work%20In%20Progress-yellow)](https://guide.unitvectorylabs.com/bestpractices/status/#work-in-progress) 

# ghook4action

A configurable rules engine for GitHub webhook events delivered through Google Cloud Pub/Sub.

Consumes verified webhook messages from ghook2pubsub, evaluates explicit repository rules, and adds matching issues and pull requests to GitHub organization projects. No privileged pull request workflow is required.

Configure a dedicated Pub/Sub subscription, a mounted rules file, and GitHub App credentials (or a static token). See [configuration](docs/configuration.md), [architecture](docs/architecture.md), and [operations](docs/operations.md). [Example rules](examples/rules.json) route public and private repositories to separate projects.

```sh
go test ./...
go run . --rules-file examples/rules.json --check-config
```

The snapshot image is `ghcr.io/unitvectory-labs/ghook4action-snapshot:dev`, built for AMD64 and ARM64. Live issue and pull request routing has been verified, including separate public/private project destinations. Standard snapshot builds trigger automatic deployment rollouts.
