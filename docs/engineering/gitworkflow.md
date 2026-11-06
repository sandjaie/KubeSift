# Git workflow

Branching, commits, pull requests, and publication for KubeSift. The engineering playbook and `AGENTS.md` point here. Following this file does not authorize a push or a release.

## Branches

- `develop` integrates. `main` is only what has been released.
- `main` moves through `release/*` or `hotfix/*`. Everything else merges to `develop`.
- The repository today has only `main`, and the commits on it are planning. Create `develop` from `main` when implementation starts. After that, feature and fix work is branched.
- Feature and fix work merges to `develop`. Do not commit that work straight to `develop` or `main`.

Documentation bootstrap exception: until implementation starts and `develop` exists, authorized planning/documentation commits may remain on `main`. This does not represent a released application. The first implementation session creates `develop` from the reviewed planning commit and uses a `feature/*` branch.

## Names

- Branch prefixes: `feature/*`, `fix/*`, `release/*`, `hotfix/*`. Kebab-case. A ticket id may prefix the description.
- These names are mandatory by repository convention. GitHub ruleset enforcement is planned, not yet verified; configure and verify a ruleset separately before claiming enforcement.

## Commits

- Conventional Commits: `feat:`, `fix:`, `chore:`, `refactor:`, `docs:`, `test:`, `perf:`, `style:`, `ci:`. The branch prefix and the commit type are independent.
- Git does not reject a message that breaks that style. There is no `commit-msg` hook.
- Release notes come from commits since the last tag. `feat` → Added, `fix` → Fixed, `refactor` / `perf` / `style` → Changed, `docs` → Documentation. `chore`, `test`, and `ci` stay in the pull request commit list and stay out of the changelog body.

## Pull requests

- Pull requests on [sandjaie/KubeSift](https://github.com/sandjaie/KubeSift) carry a summary and a test plan. Fill the template.
- Squash-merge and delete the source branch.

## Identity and publication

- Use the configured repository author and authorized GitHub account. Destination: [sandjaie/KubeSift](https://github.com/sandjaie/KubeSift).
- Before commits, verify repository-local author identity. Before remote operations, verify remote destination and authentication account separately.
- Do not infer identity from the OS username or change global Git identity. Confirm an ambiguous account before remote operations.
- The maintainer has authorized committing and pushing completed task changes by default. After appropriate verification, commit only the task’s changes and push the current authorized feature branch; preserve unrelated work. Report the commit and push result.
- This standing authorization does not authorize merges, releases, force pushes or publication outside the configured repository; obtain explicit authorization for those actions.
- KubeSift is licensed under Apache-2.0; retain the root LICENSE and required third-party notices.
- Signing, notarization, and the updater stay open until the release-readiness gate in the roadmap.

## Releases and hotfixes

- Cut `release/vX.Y.Z` from an up-to-date `develop`.
- Bump the version, update `CHANGELOG.md`, and allow only stabilization fixes on that branch.
- Open a pull request into `main` with the release template. After merge: annotated tag, GitHub release of the artifact you actually ship, merge `main` back into `develop`, delete the release branch.
- A hotfix branches from `main`, lands on `main`, gets tagged, then merges back to `develop`.
- Version, changelog, tag, and publish are a deliberate release. They are not a side effect of CI.
