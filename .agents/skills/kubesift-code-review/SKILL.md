---
name: kubesift-code-review
description: Use when reviewing KubeSift code changes for correctness.
---

# kubesift-code-review

Read AGENTS.md, docs/engineering/engineering.html and the approved task specification. Review the complete diff and its callers. Trace explicit connection/generation/resource/session targets, helper execution and refresh trust, cancellation/setup races, bounded delivery, stale events, single-use expiring mutation tokens, UID/version checks and uncertain writes. Keep Go domain services independent of Wails. Use only synthetic evidence; do not access clusters to review code.
Return actionable findings as KSF-001, severity, file:line, trigger, consequence, evidence and smallest correction. Separate validation gaps and unperformed checks from defects. Return no findings when appropriate. Review never authorizes merge or publication.
