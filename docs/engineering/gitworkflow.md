# Git workflow

Branching, commits, pull requests, and publication for KubeSift. The engineering playbook and `AGENTS.md` point here. Following this file does not authorize a push or a release.

## Branches

- `develop` integrates. `main` is only what has been released.
- Never delete `main`, `master` or `develop`. Only merged task/phase source branches are eligible for cleanup.
- `main` moves through `release/*` or `hotfix/*`. Other work targets `develop` under the rules below.
- Implementation feature and fix work merges to `develop` through the pull-request workflow. Do not commit implementation work straight to `develop` or `main`.
- Planning-only changes from the planning session, including specifications, implementation plans, architecture, roadmap and related documentation, are committed and pushed directly to `develop`. No separate planning branch or pull request is required. Run the applicable documentation checks first.
- If an implementation agent is using another branch in the shared checkout, use an isolated checkout for planning updates; never switch, stash or modify its in-progress work. Implementation branches incorporate the latest reviewed planning changes from `develop` before dependent work.

This planning exception does not authorize application code changes, merges to `main`, releases or force pushes. Existing implementation branches continue through the normal review and phase-completion gates.

## Names

- Branch prefixes: `feature/*`, `fix/*`, `release/*`, `hotfix/*`. Kebab-case. A ticket id may prefix the description.
- These names are mandatory by repository convention. GitHub ruleset enforcement is planned, not yet verified; configure and verify a ruleset separately before claiming enforcement.

## Commits

- Conventional Commits: `feat:`, `fix:`, `chore:`, `refactor:`, `docs:`, `test:`, `perf:`, `style:`, `ci:`. The branch prefix and the commit type are independent.
- Git does not reject a message that breaks that style. There is no `commit-msg` hook.
- Release notes come from commits since the last tag. `feat` → Added, `fix` → Fixed, `refactor` / `perf` / `style` → Changed, `docs` → Documentation. `chore`, `test`, and `ci` stay in the pull request commit list and stay out of the changelog body.

## Pull requests

- Pull requests on [sandjaie/KubeSift](https://github.com/sandjaie/KubeSift) carry a summary and a test plan. Fill the template.
- At the end of every completed phase, after its acceptance evidence and independent review are complete, create a pull request from the phase feature branch targeting `develop`. Include the actual checks, review resolutions and remaining limitations.
- The maintainer has authorized squash-merging completed phase pull requests into `develop` after the exact PR head has passed applicable checks and review; delete the source branch after merge. Do not bypass failed checks or unresolved blocking findings.
- This phase-completion rule does not authorize merging to `main`, starting the next phase, force pushes or releases.

## Identity and publication

- Use the configured repository author and authorized GitHub account. Destination: [sandjaie/KubeSift](https://github.com/sandjaie/KubeSift).
- Before commits, verify repository-local author identity. Before remote operations, verify remote destination and authentication account separately.
- Do not infer identity from the OS username or change global Git identity. Confirm an ambiguous account before remote operations.
- The maintainer has authorized committing and pushing completed task changes by default. After appropriate verification, commit only the task’s changes and push `develop` for planning-only changes or the authorized feature branch for implementation; preserve unrelated work. Report the commit and push result.
- This standing authorization permits completed-phase merges into `develop` only under the pull-request rule above. Other merges, releases, force pushes and publication outside the configured repository require explicit authorization.
- KubeSift is licensed under Apache-2.0; retain the root LICENSE and required third-party notices.
- Signing, notarization, and the updater stay open until the release-readiness gate in the roadmap.

## Releases and hotfixes

- Cut `release/vX.Y.Z` from an up-to-date `develop`.
- Bump the version, update `CHANGELOG.md`, and allow only stabilization fixes on that branch.
- Open a pull request into `main` with the release template. After merge: annotated tag, GitHub release of the artifact you actually ship, merge `main` back into `develop`, delete the release branch.
- A hotfix branches from `main`, lands on `main`, gets tagged, then merges back to `develop`.
- Version, changelog, tag, and publish are a deliberate release. They are not a side effect of CI.
