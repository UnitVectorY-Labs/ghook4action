# Rollout status

- Infrastructure PR https://github.com/UnitVectorY-Labs/unitvectory-labs-management-tofu/pull/123 is merged and applied. It grants subscription-scoped consumer access and retry backoff on the existing filtered subscription.
- Repository standards PR https://github.com/UnitVectorY-Labs/uvy-labs-gitrepoforge/pull/20 retires the legacy workflow with a managed deletion rule. It also removes the template. No opt-out keys are required.
- The standard snapshot workflow includes automatic rollout triggering, unchanged from other Go Docker applications.
- GitHub App authentication renews installation tokens automatically. Production configuration uses the organization's installation and a mounted private key.
- Live verification precedes removal of legacy workflows in other repositories.

## Live verification — November 5, 2026

The application is deployed to the existing production namespace alongside the audit consumer. Its own [test issue #2](https://github.com/UnitVectorY-Labs/ghook4action/issues/2) was automatically added to project 41 with no legacy workflow in this repository. Public pull request routing to project 41 and private pull request routing to project 42 were independently checked through the App's GraphQL access.

The snapshot workflow's standard rollout notification succeeded, and the cluster pulled the resulting snapshot with the expected application version. The audit consumer remains independently available on its own subscription. Legacy workflow removal PRs use the shared branch `retire-add-to-project-workflow`; they are intentionally left for owner review and merge.
