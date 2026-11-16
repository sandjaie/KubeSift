# Phase 1D.4 — YAML editing

**Status:** Simplified following user feedback; design for review, no implementation authorized.
**Date:** 2026-11-16

Owners: [roadmap](../../roadmap/roadmap.html#phase-1d), [security](../../security/security-model.html), [UX](../../ux/workflows.html), [engineering](../../engineering/engineering.html#agent-verification). Preserve the current UI and reuse existing operation infrastructure.

## What the user wants

Edit existing Kubernetes resources, including Secrets, using the permissions already granted to the user's cluster credentials. **Kubernetes decides authorization; KubeSift does not impose its own resource allowlist, role policy or extra permission approvals.** Support built-in resources, namespaced and cluster-scoped resources, CRDs and installed custom/OpenShift resources through the same editor.

The normal flow is **Edit YAML → Review diff and dry-run result → Confirm → Result**. Keep it simple and consistent across resource types. A Secret does not require a separate approval workflow, typed-name confirmation or repeated reveal gates.

This increment edits one existing object at a time. Creation, multi-document/bulk changes, server-side apply and separate subresource operations remain separate work. This scope does not exclude ordinary resource kinds from editing.

## Permissions and server behavior

Use the current connection's credentials. API discovery identifies the resource route and supported verbs. Existing RBAC checks may help show whether Edit is available, but must not become a second authorization system: an unknown permission check is not a denial. Actual get, dry-run and patch responses are authoritative. Explain Forbidden, unsupported operation, immutable field, validation error and conflict accurately.

KubeSift does not reject edits simply because they concern Secrets, RBAC, storage, cluster-scoped objects, finalizers, owner references or a terminating object. The API server decides which field changes it accepts. There is no special typed-name requirement for these YAML edits. Existing Delete-action confirmations are unchanged.

The object being edited cannot silently become a different object: its route, name, namespace, API identity and captured UID/version are fixed for this edit session. Changing those identifiers is not renaming or creating a resource. Server-owned fields and status may be shown as context but are excluded from the normal editable projection; do not generate deletions for fields omitted by the viewer. Separate status/subresource operations are outside this editor's scope. Ordinary configuration changes remain subject to server validation, not local kind-specific policy.

## Existing UI and editing behavior

Add Edit to the current YAML section, keeping its layout, colours, navigation and keyboard conventions. Fetch a fresh full object for the selected identity; never edit a summary or truncated YAML. Show the selected cluster, namespace/cluster scope and resource name. Provide a straightforward YAML editor with Cancel and Review changes.

Keep drafts in memory. Warn before discarding unsaved changes; do not lose them silently on navigation. A draft stays bound to its original resource when tabs change. Disconnect or changed connection generation invalidates a prepared submission; reconnecting requires a fresh review. No draft persistence, automatic replay or automatic rebase. Reuse existing bounded state and request limits rather than build a draft-management subsystem; concrete component structure is an implementation-plan decision.

Parse and validate YAML natively. Require one object, reject ambiguous duplicate keys and invalid syntax, preserve exact numeric values and unknown fields, and bound parsing and payload sizes to the existing 2 MiB resource limit. Do not execute templates, tags or external references. No changes means no write.

## Diff, dry-run and save

The user chose conditional JSON Patch for existing objects. Compare the complete captured object with the edited representation, preserve untouched fields, and generate only the changes plus UID/resourceVersion test conditions. Arrays may use whole-array replacement for a predictable first implementation. Use lossless conversion and correct JSON Pointer escaping.

Show one review with the user's diff, server dry-run outcome and any additional server changes/warnings. Additional server changes can be expanded in that same review; do not require a second review wizard. Use normal Kubernetes dry-run and supported validation, with no bypass on failure. Unsupported dry-run/patch must produce an understandable API limitation rather than switching to an unconditional update. [Kubernetes API concepts](https://kubernetes.io/docs/reference/using-api/api-concepts/) define conditional patch and dry-run behavior.

A successful dry-run enables one ordinary Confirm action. No typed resource names or extra confirmations based on resource kind. Changing the draft invalidates the previous review. Reuse existing native review binding, expiry, shared operation locks and single-use execution; send exactly the reviewed patch once. Do not introduce another permission or approval layer.

If the resource changed or was replaced, explain the conflict and let the user refresh/review again. Do not overwrite concurrent changes or automatically replay an uncertain request. Retain existing accepted/rejected/unknown result handling. After success refresh the object; API acceptance is not a guarantee of workload health or permanent controller state. Dry-run previews may differ from later admission or reconciliation.

## Secrets use the same workflow

Secret lists remain metadata-only. Selecting Edit or Reveal YAML explicitly requests the selected Secret's content using the user's credentials. If authorized, show and edit its YAML directly; if denied, show the server's permission error. No additional permission prompt, typed-name step or second reveal gate before reviewing the edit.

Use standard Secret YAML: `data` contains base64, and `stringData` can be supplied for text. Base64 is not encryption. Normalize stringData into data with stringData precedence before generating the patch; do not treat it as a persistent server field. Do not silently decode binary data. Kubernetes enforces immutable Secret restrictions.

The editor and review show the actual content the user deliberately opened so changes can be understood. Never substitute masked placeholders into the patch. Copy is user-initiated, with no automatic export. Closing the edit/reveal clears its application references. Secret content must not spill into another resource's UI.

For all resource kinds, keep drafts and payloads local and memory-only; never log YAML, patches or Secret values, persist them in settings/operation history, or send them to AI. Display useful server validation errors within the editing flow, but do not copy raw potentially sensitive errors into diagnostics or retained operation records. This is data handling, not an extra restriction on the user's cluster permissions.

## Implementation boundaries

Reuse existing Go discovery, connection, bounded transport and operation coordinator. Go owns route/identity checks, parsing, patch generation, dry-run and submission. React owns the editor and review presentation. Fixed and generic resource views must use the same editing behavior; default generic browsing must not fetch Secret bodies unintentionally.

The plan chooses focused files/contracts and reuses current limits and UI primitives. No new permission engine, resource-specific policy matrix, impact classifier, multi-step review wizard or editor framework without demonstrated need. Do not redesign the application.

## Minimal verification

Extend existing tests with a small representative group covering patch correctness, invalid YAML, stale UID/version, dry-run rejection, server permission denial and Secret stringData handling. Add a focused UI state check for target retention and review invalidation. No per-resource duplicated suites, snapshot collection or coverage target.

Use only kind-dev-local-a/b and synthetic owned fixtures. Exercise a workload, ConfigMap, Secret and one generic/cluster-scope example through the common path; cover networking with a representative fixture or existing integration extension. Prove one allowed edit and one RBAC-denied edit, plus conflict handling. No new clusters or infrastructure and no changes to shared system resources.

The implementing agent verifies the packaged edit/review/confirm journey, including a Secret, with the current UI, and records actual checks in `docs/superpowers/validation/phase-1d4.html`. Follow existing repository checks, independent whole-PR review and Git workflow; no duplicate user UAT checklist by default.

Acceptance:

- Built-in and discovered existing resource kinds share the editor, governed by Kubernetes permissions and validation.
- A reviewed diff and successful dry-run precede confirmation; confirmed changes target the original UID/version.
- Secrets are directly editable after the user opens them, with no extra approval layers or payload logging.
- Conflicts, denial and unknown outcomes are clear; no silent overwrite or automatic retry.
- Current UI is preserved and representative tests/native verification pass.

## Next step

Approve the simplified specification, then create the Superpowers implementation plan. No implementation, fixtures, release or cluster mutation occurs during this planning update.
