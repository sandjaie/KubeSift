---
name: kubesift-kubernetes-development
description: Use when planning or implementing KubeSift Kubernetes resource readers, relationships, API scope, status presentation, or integration tests. Also applies to debugging UID, pagination, permission and stale-response behavior; not general cluster deployment or purely visual styling.
---

# KubeSift Kubernetes development

Use this as a reference guide for building a Kubernetes client application. Product requirements and safeguards remain in their owning documents; this skill does not grant implementation, cluster mutation, publication or release permission.

## Find the right evidence

Start with repository instructions, [planning index](../../../docs/index.html) and the active spec/plan. Compare the checkpoint with the actual branch and relevant source: concurrent implementation can be ahead of documentation. Use the existing checkout; create a worktree only on explicit user request.

Open the [Kubernetes development index](../../../docs/engineering/kubernetes-development.html), then read only the rows relevant to the task:

| Task | Index section |
| --- | --- |
| New reader, scope or API-version change | API contracts and versions |
| Parent/child or named-reference navigation | Identity and relationships |
| Status, readiness or network presentation | Resource semantics |
| Tests and acceptance | Testing and evidence |

For current library/API questions, use the repository's Context7 lookup instructions; inspect dependency versions in go.mod and supported server behavior before adopting newer fields. The index is a map, not a cached API contract or authorization to install tools.

## Apply the references to the change

Trace the relevant path from Go reader/DTO through desktop adapter, generated bindings, frontend state and UI, including existing callers and tests. The index maps these locations. Capture the target kind/API/scope, identity and evidence meaning before changing code; use the active specification for exact limits and acceptance criteria.

Keep three questions separate: what the API returned, what KubeSift may infer from it, and what the test actually proves. Use official references to settle API semantics and owning documents to settle product choices. Update the owning document when a choice changes; add only durable reference links here or in the index.

Example: a Service label on an EndpointSlice establishes association; consult ownership/reference documentation and the networking spec before presenting that as ownership. A same-name Pod lookup cannot establish historical UID identity. Trace the reference-validation reader and navigation tests, rather than replacing the check with label or IP matching.

## Choose verification evidence

Use [testing and evidence](../../../docs/engineering/kubernetes-development.html#testing) to distinguish unit fakes, API-server integration and native UI observations. Follow the documented local fixture setup and [agent verification policy](../../../docs/engineering/engineering.html#agent-verification). Runtime checks are part of authorized implementation, not a side effect of reading this skill or performing a code review.

Report actual outcomes and remaining limits. For planning deliverables, include the user's copy-ready separate-session implementation handoff. For code review, use the repository's correctness-review skill; this reference guide does not replace independent review.
