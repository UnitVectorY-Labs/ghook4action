# Initial rollout status

- Application and configuration are prepared independently of destination project constants.
- Infrastructure draft PR: https://github.com/UnitVectorY-Labs/unitvectory-labs-management-tofu/pull/123. Reuses existing filtered subscription and grants only its subscriber role to the new workload identity.
- Repository standards draft PR: https://github.com/UnitVectorY-Labs/uvy-labs-gitrepoforge/pull/20. Adds `add_to_project: false` and `docker_snapshot_rollout: false` opt-outs. Until merged and the local templates updated, the regular config checkout does not recognize this new key. The app was generated using the isolated PR worktree.
- The initial snapshot workflow disables automatic cluster rollout using `docker_snapshot_rollout: false` while deployment remains unapproved.
- Deployment manifests remain local to the Talos checkout, outside the normal apply-all path until approved. No live cluster resource is provisioned by building this repo.
- Legacy workflows in other repositories remain in place until live routing is verified.
