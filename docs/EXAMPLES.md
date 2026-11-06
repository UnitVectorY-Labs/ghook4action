---
layout: default
title: Examples
nav_order: 5
permalink: /examples
---

# Examples

## Route selected public repositories

Save this as `rules.json`, replace `YOUR_ORG`, the repository names, and project number, then run the offline validation command in [Usage](USAGE.md).

```json
{
  "version": 1,
  "rules": [
    {
      "name": "selected-public-repositories",
      "match": {
        "events": ["issues", "pull_request"],
        "actions": ["opened"],
        "repository_owner": "YOUR_ORG",
        "repository_private": false,
        "repositories": ["service-one", "service-two"]
      },
      "actions": [
        {"type": "add_to_project", "project": {"owner": "YOUR_ORG", "number": 1}}
      ]
    }
  ]
}
```

Use the number in the organization project's URL, not a GraphQL node ID. Omit `repositories` to match all repositories of the specified visibility in that organization. Repository owner/name matching is case-insensitive.

## Separate public and private destinations

The checked-in [example rules](../examples/rules.json) contain two rules: public repositories route to one project, private repositories to another. Replace both organization names and project numbers before use. `repository_private` must explicitly be `true` or `false`; there is no wildcard. For pull requests, visibility refers to the base repository, not a contributor's fork.

## Multiple projects

Add another `add_to_project` entry to a rule's `actions` array to send a matching item to multiple projects. All matching rules execute in file order; processing does not stop after the first match. If a later action fails, the message retries and earlier successful additions may run again safely.

Unknown keys, unsupported event/action types, and missing required fields fail validation. Rules contain data only: scripts and arbitrary expressions are not supported.

## Trigger workflows across repositories

See [Workflow orchestration](WORKFLOWS.md) for a complete upstream CI to downstream dispatch example, an optional branch-push rule, and the downstream workflow contract. The [workflow rules example](../examples/workflows.json) can share one configuration's `rules` array with the project examples. Multiple dispatch actions fan out to multiple destinations; every matching rule runs.
