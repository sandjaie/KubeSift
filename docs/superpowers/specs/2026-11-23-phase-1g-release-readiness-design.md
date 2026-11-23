# Phase 1G · macOS release readiness

Planning design, 23 November 2026. The [roadmap](../../roadmap/roadmap.html#phase-1g) owns the phase exit. The [Git workflow](../../engineering/gitworkflow.md) owns publication.

## Download, install and upgrade

The maintainer is the first supported user. A public GitHub Releases ZIP should let the maintainer install KubeSift without building it. Use the existing architecture-labelled ZIP, bundled license and notices, and SHA-256 checksum. Start with the maintainer’s actual Mac and OS version. Record both during verification and limit the initial support claim to the configuration exercised. Record the configured deployment target separately.

Use Developer ID Application signing and Apple notarization for the GitHub ZIP. The maintainer performs both on their personal Mac with local Keychain credentials. Local development keeps its existing ad-hoc packaging. Preserve quarantine and require normal Gatekeeper acceptance on first launch. Use manual upgrades from GitHub and defer an in-app updater. See [Apple’s notarization workflow](https://developer.apple.com/documentation/security/customizing-the-notarization-workflow).

Rehearse with a browser download of the existing release and an isolated macOS user profile. Install in the isolated user’s ~/Applications, launch from Finder, connect to an approved kind context, save a harmless preference and close the app. Upgrade the isolated installation to the release candidate and verify that the preference and connection configuration still work. Record the actual versions, architecture, archive digest, signature, first-launch prompts and observed results. Preserve the maintainer’s everyday app, preferences and credentials.

For the 1G readiness decision, rehearse on develop. In a second clean profile on the target Mac, browser-download the candidate from a controlled staging location, install in ~/Applications and launch from Finder without prior app approval or settings. Use the same candidate archive for the separate upgrade check. Compare digests and record results independently.

Tagging starts only after separate release authorization. Follow the existing release workflow to package the tagged source and smoke-test the bundle. Before uploading, repeat clean install and upgrade with the archived tagged bundle downloaded through the browser. Retain those exact bytes for upload. After publication, download the public GitHub asset, verify its digest and repeat Finder launch. These tagged and public-asset checks belong to release execution. Readiness can be assessed with the develop rehearsal. Record a blocked result if the required environment or approval path is unavailable.

## Capability and compatibility decisions

Map each Phase 1 requirement to the existing [capability matrix](../../product/feature-matrix.html) and its strongest evidence. For bulk actions, local shell, file copy and extensions, record an explicit include or defer decision. Record the consequence of each omission for the user. Additional feature implementation requires its own approved scope.

Carry forward the [live OpenShift findings](../validation/phase-1f.html#live-2026-11-23) and [Projects results](../validation/phase-1f1.html#live-2026-11-23). PackageManifest list and lookup are observed defects. Restricted Project selection passed. OpenShift version confirmation, successful Route and Build lifecycle operations, and Operator lifecycle verification remain open. Keep RBAC denials distinct from application defects. Phase 1G planning may proceed alongside 1F. Phase closure requires the roadmap dependencies to pass or an explicit maintainer-approved change to the owning scope.

## Operational evidence and release decision

Reuse versioned evidence where it covers the proposed support claim. For each remaining check identify the fixture, expected user-visible outcome, and evidence needed. Cover Kubernetes and OpenShift versions, EKS/AKS/oc credential renewal and missing helpers, reconnect and sleep/wake, monitor changes, keyboard and accessibility behavior, and large inventories and streams. Use approved local kind contexts for development checks. External tests require separately identified targets and permitted actions. Record unavailable environments as unverified.

Review credential storage, diagnostic and log redaction, dependency notices and the contents of the exact archive. Keep credentials and private cluster details out of reports. Keep detailed working evidence in the ignored local ledger and summarize observed outcomes in the phase validation report.

Deliver an evidence-linked capability and platform matrix, a release checklist, and user-facing installation, upgrade and troubleshooting instructions. Mark each release gate passed, failed or unverified with its consequence. Preserve the existing release workflow and require separate authorization to tag or publish.
