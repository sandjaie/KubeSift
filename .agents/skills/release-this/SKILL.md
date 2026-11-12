---
name: release-this
argument-hint: "[x.y.z | patch | minor | major]"
description: Use when asked to release KubeSift, cut a release, or ship a version, including release notes, version bump, git tag and GitHub release. Implements the release process linked from CONTRIBUTING.md.
---

# Release KubeSift

An invocation to perform a release authorizes completing the process once CI and required release gates pass, including merge, tag, artifact publication, back-merge and cleanup. Do not ask for merge confirmation. A request to edit this skill or discuss release notes is not a release invocation.

Read [CONTRIBUTING.md](../../../CONTRIBUTING.md), then execute the canonical [Git workflow release process](../../../docs/engineering/gitworkflow.md#releases-and-hotfixes). Read the [engineering release gate](../../../docs/engineering/engineering.html#release) and linked roadmap readiness requirements. Keep the procedure in its owning document rather than duplicating it here.

- Before release mutations, require a clean checkout and up-to-date `develop`; verify author, destination and authenticated GitHub account separately.
- Accept an explicit version or `patch`, `minor`, `major`. Without one, ask whether this is minor or patch. Never guess from commits.
- Use the shipped plist version as the baseline, synchronizing Wails/frontend/lockfile versions. Root `package.json` has no version.
- Run the documented checks, open the release PR into `main`, and follow it to merge. Failed checks, closed PRs and timeouts stop the release; none authorize proceeding to a tag.
- After merge, preserve the order: annotated tag → Wails package → packaged-app smoke → GitHub release with tested asset → back-merge → branch cleanup. Failed or unrun package/smoke blocks publication even if the tag already exists.
- Resume by inspecting existing state. Never move a published tag, overwrite an unverified asset, bypass CI or claim unrun checks passed.
- Report tag/release links, artifact/checksum, evidence and actual branch synchronization. Preserve signing/notarization/updater limitations.

This repository uses GitHub, develop/main, and a macOS Wails bundle. Do not import another project's GitLab, Node SEA, Helm smoke, installer or artifact commands. Only add a .claude/skills/release-this directory symlink to this canonical skill directory if the repository adopts Claude skill loading; never maintain a second copy.
