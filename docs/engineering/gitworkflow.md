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
- For phase implementation tasks, include the phase and task number in the subject, for example `feat: phase1c2-task1 add bounded workload inventory reads`. Use the actual phase identifier (such as `phase1c-task1` for Phase 1C task 1). When a commit contains several substantial updates, add a body with concise bullet points explaining the changes and relevant validation.
- Git does not reject a message that breaks that style. There is no `commit-msg` hook.
- Release notes come from commits since the last tag. `feat` → Added, `fix` → Fixed, `refactor` / `perf` / `style` → Changed, `docs` → Documentation. `chore`, `test`, and `ci` stay in the pull request commit list and stay out of the changelog body.

## Pull requests

- Acceptance evidence follows [agent verification](engineering.html#agent-verification). Agent-run desktop checks satisfy the normal UI verification requirement; a duplicate manual user UAT checklist is not a merge gate. Unresolved failures, material verification gaps and explicit authorization requirements still apply.

- Pull requests on [sandjaie/KubeSift](https://github.com/sandjaie/KubeSift) carry a summary and a test plan. Fill the template.
- At the end of every completed phase, after its acceptance evidence and independent review are complete, create a pull request from the phase feature branch targeting `develop`. Include the actual checks, review resolutions and remaining limitations.
- GitHub CI runs only for release-producing PRs targeting `main` from `release/*` or `hotfix/*`. Feature PRs targeting `develop` and push events do not trigger CI. Applicable local validation and independent review remain required for feature work.
- The maintainer has authorized squash-merging completed phase pull requests into `develop` after the exact PR head has passed applicable checks and review; delete the source branch after merge. Do not bypass failed checks or unresolved blocking findings.
- This phase-completion rule does not authorize merging to `main`, starting the next phase, force pushes or releases.

## Identity and publication

- Use the configured repository author and authorized GitHub account. Destination: [sandjaie/KubeSift](https://github.com/sandjaie/KubeSift).
- Before commits, verify repository-local author identity. Before remote operations, verify remote destination and authentication account separately.
- Do not infer identity from the OS username or change global Git identity. Confirm an ambiguous account before remote operations.
- The maintainer has authorized committing and pushing completed task changes by default. After appropriate verification, commit only the task’s changes and push `develop` for planning-only changes or the authorized feature branch for implementation; preserve unrelated work. Report the commit and push result.
- This standing authorization permits completed-phase merges into `develop` only under the pull-request rule above. Other merges and releases require explicit authorization, including invocation of `release-this` as described below. Force pushes and publication outside the configured repository require separate explicit authorization.
- KubeSift is licensed under Apache-2.0; retain the root LICENSE and required third-party notices.
- Signing, notarization, and the updater stay open until the release-readiness gate in the roadmap.

## Releases and hotfixes

The canonical agent entry point is [.agents/skills/release-this/SKILL.md](../../.agents/skills/release-this/SKILL.md). Asking an agent to “release this” or invoking `/release-this` authorizes the release branch push, PR into `main`, merge after successful CI, annotated tag, tested artifact publication, back-merge and release-branch cleanup. Do not ask again for merge confirmation. Editing the skill or discussing a release does not invoke it. Existing acceptance and release-readiness requirements still apply.

### Before cutting a release

- Require a clean working tree before switching branches; never stash or discard someone else's work. Verify local author, remote destination and authenticated GitHub account independently under Identity and publication.
- Run `git checkout develop`, `git pull --ff-only origin develop`, and `git fetch origin main --tags`. Require local `develop` to match `origin/develop`; stop on divergence. Do not push unrelated local commits.
- Accept `X.Y.Z`, `patch`, `minor`, or `major`. Without an argument, ask “Is this a minor or patch release?” Never infer a bump from commit history. Require an explicit version to exceed the current version using semantic version comparison.
- The shipped macOS version is `CFBundleShortVersionString` in `build/darwin/Info.plist`; use it as the current version. The root `package.json` has no version. Keep both plist version keys, `build/config.yml` → `info.version`, and `frontend/package.json` plus its root `package-lock.json` workspace metadata aligned to the selected version. Do not change versions while merely updating this procedure.
- Read the engineering release gate and roadmap release-readiness requirements. Current packaging is ad-hoc signed, with signing/notarization/updater work still open. Do not represent local validation as distribution readiness. Resolve missing required readiness evidence before merging/tagging.

### Phase A — cut the release

1. Find the last version tag with `git tag -l 'v*' --sort=-v:refname | head -1`. Verify it belongs to the integration history; stop on unrelated or ambiguous tag history. Read subjects with `git log <last-tag>..develop --pretty=format:'%s' --no-merges`. With no tag, use `git log develop --pretty=format:'%s' --no-merges`, including the root commit. Capture a separate complete commit list, including merges, for the PR.
2. Map Conventional Commit subjects as described under Commits. Strip the type, optional scope and breaking-change marker, preserving the remaining subject verbatim. Omit empty sections. Inspect unclassified subjects rather than silently dropping user-facing changes. Prepend a dated `X.Y.Z` entry to `CHANGELOG.md`, creating a Keep a Changelog header if missing. Use the user's current local date.
3. Create `release/vX.Y.Z` from `develop`. Run `npm version X.Y.Z --workspace frontend --no-git-tag-version` and synchronize the Wails/plist fields above. Verify the lockfile and exact diff. Do not create a tag yet.
4. Run `wails3 task bindings`, `npm run build`, and `scripts/check.sh` (source descriptions, documentation links, Go formatting/tests/vet, frontend tests/lint/typecheck). Also satisfy applicable acceptance gates. Generated changes outside release metadata must be investigated and handled separately, never silently staged or discarded.
5. Commit only the version files and `CHANGELOG.md` as `chore: release X.Y.Z`. Push the release branch to `origin`. Create the PR with `gh pr create --repo sandjaie/KubeSift --base main --head release/vX.Y.Z --title "chore: release X.Y.Z" --body-file <prepared-file>`. Fill `.github/pull_request_template.md` (there is currently no separate release template), adding release notes, every commit in the range, actual check results and artifact/readiness limitations.
6. Use `gh pr merge --auto --merge` for the release PR so integration ancestry is preserved. Require successful CI on its exact head and all repository review requirements; never use an administrative bypass. If auto-merge is unavailable, watch checks and merge normally only after they pass. Missing/pending checks are not success.
7. Poll PR state and checks about every 15 seconds for at most 30 minutes in a resumable long-running command, yielding output for progress updates. On failed CI, report its URL and stop for a fix on the release branch. Recheck the new head after a fix. On closure without merge or timeout, report the state and stop; do not start Phase B. Once merged, continue immediately.

### Phase B — tag, package, smoke, publish, back-merge

1. Check out `main` and pull with `--ff-only`. Verify its head is the intended PR merge commit; stop if another release has advanced it. Create annotated `vX.Y.Z` with this release's notes and push that tag to `origin`. On resume, verify any existing tag targets that exact commit; never move or replace a published tag.
2. Run `wails3 task package`, producing `bin/kubesift.app`. Packaging is mandatory for this desktop app. On failure stop before creating/updating a GitHub release or uploading anything. Retain the tag and report the resumable state.
3. Smoke-test that exact packaged app, including Finder launch and applicable user journeys from the phase validation checklist. Record the tested desktop and integration versions and explicit local kind contexts. There is no general automated release-smoke command: historical measurements or unit tests do not substitute for this smoke. If it fails or cannot be performed, stop before publication. Preserve evidence in ignored `.superpowers/sdd/`.
4. Archive the successfully tested bundle using macOS `ditto -c -k --sequesterRsrc --keepParent bin/kubesift.app <asset.zip>`, with version and actual build architecture in the asset name. Verify LICENSE/NOTICE are included and record a SHA-256 checksum. Do not rebuild between smoke and archive. Publish with `gh release create vX.Y.Z <asset.zip> --verify-tag --title "KubeSift X.Y.Z" --notes-file <release-notes-file> --repo sandjaie/KubeSift`. Include actual signing/notarization limitations. If the release already exists, inspect it and upload a missing verified asset with `gh release upload`; do not overwrite a different existing asset or upload after failed smoke.
5. Check out `develop`, pull with `--ff-only`, merge `origin/main` and push `develop`. Preserve concurrent integration work; stop on conflicts needing product decisions. Verify `main` is an ancestor of `develop` and version/changelog content agrees. Report whether branch tips match exactly; do not claim equality when later integration work exists.
6. Only after successful publication and back-merge, delete the remote release branch and its merged local branch. Never delete `main` or `develop`.

Report the tag URL, release URL, asset name/checksum, validation results and branch synchronization status. On failure report the last completed step and remaining work; reruns inspect existing PR/tag/release state before creating anything.

An explicitly requested hotfix starts from `main`, uses `hotfix/*`, and follows the same validation, tag, package, smoke, publication and back-merge rules. Do not infer hotfix mode.
