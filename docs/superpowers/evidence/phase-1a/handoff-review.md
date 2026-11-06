# KubeSift handoff checkpoint review

Reviewed checkpoint: `371f9a9..3b30397e82fbfa590a8b55311bde7b821f07e5c5`, using the supplied frozen diff and the corresponding source/callers in `/Users/sandjaie.ravi/dev/kubesift`. This is a checkpoint review before the user's authorized feature-branch push, not completed Task 7, Phase 1A acceptance, merge approval or release authorization.

## Strengths

- Explicit connection IDs/generations and session references survive tab changes; stale connection requests are rejected and connection-owned cancellation reaches HTTP work. Native client-go exec authentication is disabled. The owned helper adapter rechecks executable resolution/configuration and serializes authorization with process start; previews redact all argument values and environment values.
- The Go operation/session/connection services stay independent of Wails. Mutation previews copy the target, UID/resourceVersion, payload and five-minute expiry; execution consumes the token under a mutex before network work, checks the resource again, and uses `MaxRetries(0)` for the write. Errors distinguish known rejection from uncertain outcome without leaking raw credential diagnostics.
- Session queues have explicit byte/item bounds, bridge credits restrict data delivery, terminal acknowledgements occur after xterm parsing, and port forwards bind loopback. Backend cleanup callbacks release terminal resources. Cluster strings enter React as text; terminal link/clipboard escape actions are blocked.
- Tests concentrate on meaningful safety groups, one frontend test and one opt-in integration scenario. Source descriptions identify purpose, consumers, responsibilities and safeguards. No actionable dependency/scaffolding or broad refactoring request arose from the hygiene pass.
- The handoff explicitly records the interrupted 226.868-second run, lower actual generator rates, missing twenty cycles and remaining manual acceptance. It does not claim a Phase 1A pass. Existing logs support the reported completed check/race/integration/package commands; I did not rerun those commands.

## Actionable findings

### KSF005 — P1 / Important: Namespace changes leave the Pod picker backed by another namespace's watch

**Location:** `frontend/src/App.tsx:34` (related target construction at line 56 and namespace input at line 73).

**Trigger:** Start a watch in namespace A, change the namespace field to B, then select a Pod shown by the still-running A watch. The namespace input clears `tab.pods` and the selection, but `activeWatch` is the first watch with resources, regardless of `session.target.namespace`. It continues to override the cleared/refreshed B Pod list.

**Consequence:** The selected Pod UID, annotations and container names describe A, while `target()` combines that Pod name/container with namespace B. If B contains an identically named Pod/container, logs, forwarding or a writable container shell targets B despite the resource details having come from A. The backend correctly honors the mismatched explicit request; backend generation guards cannot repair the UI's mixed identity.

**Evidence:** The watch selection at lines 34–35 lacks a target-namespace predicate; `target()` at line 56 reads `active.namespace` independently of `selected.namespace`; `registerSession()` already stores the captured watch target. This is a deterministic source trace, not a claimed observed cluster mutation.

**Smallest correction:** Only use watch resources matching the active explicit namespace and connection; ensure selected Pod namespace matches that target before constructing a session request. Preserve old sessions in their own dock. Add a representative same-name A/B namespace regression to the existing frontend safety test, exercising the shared selection/target function used by App.

### KSF006 — P2 / Important: Validation cleanup abandons remaining owned resources after the first cleanup error

**Locations:** `frontend/src/profile.ts:20`; `scripts/measure-packaged.py:104`–107.

**Trigger (renderer):** Any `controls.stop(ref)` rejects, for example a backend cleanup timeout. `stopBatch()` clears the entire tracked list before awaiting stops, then stops iterating on the first rejection. Its outer catch retries `stopBatch()` with an empty list.

**Consequence:** Later owned watches/logs/terminal/forward sessions are never asked to stop, and the harness has discarded their cleanup ownership list. It can leave active sessions after reporting profile failure.

**Trigger (launcher):** The profile is interrupted during flood and B's `rate(100)` restoration fails because the Pod/server is unavailable or ownership has changed.

**Consequence:** That exception exits the `finally` block before `stop(process, log)`, leaving the owned packaged process and its other sessions/listeners alive. This is especially relevant to a validation workflow that includes network failure.

**Evidence:** The renderer's destructive list reset and sequential early rejection are at line 20; the Python `finally` has no nested protection around the process stop. Existing successful cleanup evidence does not exercise these failure branches.

**Smallest correction:** Attempt every tracked session stop even after an individual failure; retain/report failures without forgetting unprocessed ownership. Put owned process/log cleanup in a nested `finally` so failed fixture restoration cannot skip it. Preserve both errors where practical. Use small injected-failure regressions, without another cluster/profile run merely for this fix.

### KSF007 — P2 / Important: Numeric `sessionErrors` can report zero after a session failure

**Location:** `frontend/src/profile.ts:17`; related `internal/session/manager.go:132`–135 and `frontend/src/state.ts:20`–21.

**Trigger:** A worker returns an error while not cancelled. The manager emits an `error` event immediately followed by `state: ended`. The reducer retains the error text but replaces `status` with `ended`. The next five-second sample counts only views whose status is exactly `error`. Cleanup cycles also remove the views before recording their sample.

**Consequence:** A failed log/watch/terminal can be recorded as zero session errors, and errors during a cleanup cycle disappear from subsequent evidence. The retained JSON number therefore cannot establish an error-free run.

**Evidence:** The ordered error/ended event generation and reducer state assignment are direct code paths; this does not require a timing coincidence beyond sampling after both events.

**Smallest correction:** Track a cumulative observed session-error count that survives `ended` and view removal (and avoid counting duplicate stale events), then have the profile record that count. Add error→ended→removal behavior to the existing frontend safety test. Correct the historical report's interpretation: preserve raw numeric evidence unchanged, but call it the old undercounting counter rather than proof of zero errors.

**Historical qualification:** No actual failed session in the interrupted run is demonstrated by this review. The reported eight live sessions and successful RTTs are independent observations. Do not relabel historical measurements as failures; only withdraw an inference the counter cannot support.

## Acceptance gaps, separate from code defects

- The prescribed uninterrupted 600-second, calibrated 100/2000-line-per-second profile and twenty cleanup cycles remain unperformed. Actual source rates were approximately 81.887/1624.051 lines/sec, and the interrupted run ended at 226.868 seconds. No sustained leak or post-cleanup memory trend conclusion follows.
- Finder/default-source and the complete packaged helper lifecycle, detailed manual targeting/mutation review, forward conflict/normal stop, network/expired-watch recovery, sleep/wake, setup-race closure and active-session app exit remain incomplete as listed in the tracked handoff.
- Wails linker deployment-target warnings and expected client-go cancellation diagnostics remain the prior explicitly carried minor validation-quality follow-up. They are not fresh evidence of targeting failure.
- Existing tests/logs are evidence from the implementation session. No new tests, builds, package starts, clusters, mutations or fault injection were performed during this review.

## Hygiene and review limits

I read the approved Phase 1A spec, index, engineering/security/git rules, Task 6 review, Task 7 handoff, the frozen change inventory, the handwritten Go/TS/TSX service and renderer paths, representative tests, fixture lifecycle/measurement/check tooling, build configuration and relevant retained check output. The initial combined diff output was truncated; the affected handwritten source portions were recovered directly rather than regenerating the diff. No source/index/branch state was changed. This `/private/tmp` report is the sole intended review artifact.

Commands were read-only `cat`, `sed`, `rg`, `wc`, `nl`, `tail` and a Python read of the supplied frozen diff, plus writing this report. No verification suite was rerun. I did not independently repeat the parent's tracked credential-pattern scan. No concrete credential exposure was found in the reviewed source/evidence; the ignored credential/journal paths and parent's scan are supporting evidence, not a universal absence-of-secrets guarantee.

## Declined to judge

- Public-provider/OpenShift authentication compatibility and minimum-macOS/cross-platform support: explicitly unvalidated follow-up outside this local checkpoint.
- Phase 1B explorer completeness, Lens/K9s parity, AI features, release signing/notarization/updater and publication readiness: outside the approved Phase 1A scope.
- Quantitative performance budgets, leak freedom and complete fault recovery: current profile/acceptance evidence is incomplete; do not infer results from successful unit tests.
- Binary asset internals and every generated binding/lockfile line: treated as generated/build support; no independent binary/dependency supply-chain audit or generated-code semantic audit claimed.
- Broad refactoring of dense frontend formatting, exhaustive UI tests or additional framework abstractions: no demonstrated correctness need beyond the three specific findings.

## Checkpoint verdict

**Correctness review: changes required (KSF005–KSF007).** KSF005 affects safe resource targeting; KSF006 and KSF007 affect cleanup and evidence reliability. The checkpoint may be preserved as explicitly incomplete authorized handoff work, but it must not be described as correctness-approved until the focused fixes are reviewed. No credential leak requiring emergency publication containment was identified. This review grants no merge/release/Phase 1B authorization and no Phase 1A acceptance.
