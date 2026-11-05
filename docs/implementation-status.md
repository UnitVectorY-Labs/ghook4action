# Rollout status

- Infrastructure PR https://github.com/UnitVectorY-Labs/unitvectory-labs-management-tofu/pull/123 is merged and applied. It grants subscription-scoped consumer access and retry backoff on the existing filtered subscription.
- Repository standards PR https://github.com/UnitVectorY-Labs/uvy-labs-gitrepoforge/pull/20 retires the legacy workflow with a managed deletion rule. It also removes the template. No opt-out keys are required.
- The standard snapshot workflow includes automatic rollout triggering, unchanged from other Go Docker applications.
- GitHub App authentication renews installation tokens automatically. Production configuration uses the organization's installation and a mounted private key.
- Live verification precedes removal of legacy workflows in other repositories.
