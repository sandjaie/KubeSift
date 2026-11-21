---
name: kubesift-code-review
description: Use when independently reviewing a completed KubeSift pull request before merge, or rechecking fixes to its review findings. Covers the complete PR across tasks, not routine per-task reviews.
---

# kubesift-code-review

## Scope and handoff

Follow the [PR review gate](../../../docs/engineering/gitworkflow.md#independent-pr-review). The implementing agent dispatches one fresh reviewer after all work intended for the PR and applicable automated/UI verification are complete. Do not schedule reviews after each plan task. An existing PR is optional: the committed candidate must represent its complete intended diff.

The reviewer receives the repository path, target branch and base SHA, merge-base SHA, candidate head SHA, approved spec/plan paths and validation evidence. Use a fresh context, not the implementer's conversation or suggested findings. Read AGENTS.md and relevant owning documents. Independently inspect the diff from the recorded merge base to the candidate head, changed files, callers and tests. Verify the supplied identities; report missing evidence or a moving candidate instead of silently reviewing a different revision. Uncommitted edits are not part of a completed-PR review.

## Review focus

Prioritize reproducible correctness bugs, regressions, security boundaries, requirement mismatches and material verification gaps. Scale investigation to the diff; avoid speculative findings or cosmetic preferences.

Trace explicit connection/generation/resource/session targets, helper execution and refresh trust, cancellation/setup races, bounded delivery, stale events, single-use expiring mutation tokens, UID/version checks and uncertain writes. Check Kubernetes identity and relationship semantics, partial/permission/error states, and frontend navigation and async state across feature boundaries. Keep Go domain services independent of Wails. For documentation-only changes, check consistency, links, authorization and workflow consequences rather than inventing runtime tests.

Use only synthetic evidence. Do not access clusters, run credential helpers, mutate files, commit, push or merge. The reviewer reports findings; the implementer fixes them. Review does not grant publication authority.

## Return to the implementing agent

- Reviewed target/base, merge base and head SHAs; scope and evidence actually inspected.
- Findings as KSF-001 onward, with severity (P0 critical, P1 high, P2 medium, P3 low), file:line, concrete trigger, consequence, supporting evidence and smallest correction. Mark whether each finding blocks merge and why.
- Separate verification gaps and unperformed checks from demonstrated defects. Say “No findings” when appropriate; never fabricate issues to fill the report.
- Verdict: blocking findings, incomplete evidence, or no blocking findings. A review verdict is not a substitute for required tests or repository approvals.

The implementer evaluates every finding, fixes confirmed defects and reruns affected checks. Record fixed, rejected-with-evidence or deferred-with-rationale for every ID; blocking findings cannot be deferred to merge. Return disputed blockers to the reviewer for resolution. Recheck fixes and affected callers with the same reviewer, recording the new head SHA and delta from the reviewed head. These are targeted follow-ups to the single PR review, not another review per task. If later changes expand scope, review that added scope before merge. Preserve the original whole-PR report and append follow-up coverage so the final head is accounted for.

Example: after reviewing all workload-reader and UI changes together, a stale-response finding is fixed in the controller. Recheck that fix, related cancellation paths and regression evidence; retain the initial full-PR coverage instead of restarting unrelated reviews.
