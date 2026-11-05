# Configuration

Set `PUBSUB_SUBSCRIPTION` to the full `projects/PROJECT/subscriptions/SUBSCRIPTION` resource. Use a dedicated pull subscription; sharing a subscription with an audit consumer splits deliveries rather than broadcasting them.

Set `RULES_FILE` (default `rules.json`) to a mounted JSON rules file. Validate offline with `/server --rules-file /config/rules.json --check-config`. A restart is required to reload rules or a rotated token.

Supply exactly one of `GITHUB_TOKEN_FILE` (recommended mounted Secret) or `GITHUB_TOKEN`. Never put credentials in rules, command arguments, container images, or git.

`HEALTH_PORT` defaults to `8080`. CLI flags `--rules-file`, `--pubsub-subscription`, and `--health-port` override environment settings. `--version` prints the injected build version.

See [example rules](../examples/rules.json). The example routes opened issues and pull requests in UnitVectorY-Labs public repositories to project 41, and private repositories to project 42. These are examples, not application defaults.

Configuration version is `1`. Each named rule has `match` and `actions`. Match fields are ANDed; entries within `events` or `actions` are ORed. `repository_owner` and the explicit `repository_private` boolean are required. Optional `repositories` limits matching to those repository names. All matching rules execute in file order. The initial supported action is `add_to_project`, whose project contains organization `owner` and positive project `number`.

Rules are data, with no scripts, shell interpolation, remote templates, or arbitrary expressions. Unknown keys and invalid configuration fail startup. Narrow organization matching prevents unrelated webhook traffic from invoking privileged actions.
