---
name: release-this
description: Use when the user explicitly requests a KubeSift release.
---

# release-this

Read docs/engineering/gitworkflow.md and docs/engineering/engineering.html; those documents own the procedure. Verify local author, remote destination and authenticated GitHub account independently. An invocation does not authorize unspecified publication actions.
Prepare a reviewable release branch, intended version/changelog, exact artifact and test evidence. Use release/vX.Y.Z from develop, or hotfix/* from main for an explicitly requested hotfix. Run the applicable gates and packaged smoke checks. Report signing/notarization/updater limitations. Obtain authorization for any merge/tag/push/GitHub release not already authorized. After authorized release into main, create the annotated tag and GitHub release of the tested artifact, merge main back to develop and remove the release branch as the Git workflow directs. Never infer validation success or remote identity.
