# Phase 1G macOS release readiness implementation plan

**Goal** — Verify the maintainer can download KubeSift from GitHub, launch it in Finder and upgrade while retaining settings. Follow the [design](../specs/2026-11-23-phase-1g-release-readiness-design.md) and [release workflow](../../engineering/gitworkflow.md). On implementation authorization, use the executing-plans workflow. Planning grants no authority to sign, tag or publish.

## 1. Establish scope and evidence

- [ ] Reconcile each Phase 1 capability with its implementation and best available evidence in [the capability matrix](../../product/feature-matrix.html). Decide include or defer for bulk actions, local shell, file copy and extensions. Record user impact.
- [ ] Create the Phase 1G report in the validation directory and link it from the validation index. List the tested versions and hardware separately from build targets. Carry forward the recorded Phase 1F defects, RBAC limits and skipped lifecycle tests. Keep the phase dependency open until satisfied or explicitly rescoped.

## 2. Prepare the release artifact on the personal Mac

- [ ] Extend the release instructions in the Git workflow using the existing Wails package task. Record source revision, version, architecture and deployment target. Run the repository checks and inspect the package for expected license and dependency notices.
- [ ] On the personal Mac, confirm a usable Developer ID Application identity with its private key and local notarization credentials. Keep identifiers, secrets and credential setup out of shared logs. Missing credentials block the signed rehearsal.
- [ ] Sign nested executable code where present, then the app, with hardened runtime and secure timestamp. Verify the signature. Archive for submission and use notarytool with the local Keychain profile. Require an accepted result and review the log. Staple and validate the ticket on the app, create the final distribution ZIP and record its SHA-256. Check the extracted app’s signature, ticket and Gatekeeper assessment. [Apple workflow](https://developer.apple.com/documentation/security/customizing-the-notarization-workflow)

## 3. Verify installation and operations

- [ ] Browser-download the final rehearsal ZIP using the staging procedure in the design. Match its digest and check quarantine before first open. In a clean macOS profile install in ~/Applications, launch from Finder without a security override and connect to an explicitly selected approved kind context. Record normal prompts and results.
- [ ] In a separate profile, establish a previous-release installation, save a preference and configure a connection. Replace it with the same candidate, relaunch and verify both survive. Keep the original installation isolated.
- [ ] Compare existing phase evidence to each remaining roadmap check. Run uncovered reconnect, sleep/wake, missing helper, credential renewal, accessibility, monitor layout and large inventory/stream checks within authorized environments. Record fixture size, observed behavior and limitations. Inspect logs and diagnostics for credential exposure. Report unavailable provider or hardware cases individually.

## 4. Document the decision and hand off release execution

- [ ] Update the README installation, upgrade and troubleshooting guidance from the observed prompts. Record what users can download, the tested configuration, verification procedure and known issues. Run documentation and repository checks and complete the implementation review under the Git workflow.
- [ ] Record passed, failed and unverified gates in the validation report, including the 1F dependency. Update the portable checkpoint. A develop rehearsal can establish artifact readiness while overall Phase 1 closure remains pending.
- [ ] At a separately authorized release, repeat packaging, signing, notarization, clean-install and upgrade checks on the tagged source. Upload the exact tested final ZIP. Browser-download the public asset, verify the digest and Finder launch, then record the release result.
