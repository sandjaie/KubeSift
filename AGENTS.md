# KubeSift repository instructions

## Planning and scope
- Start at docs/index.html for durable status and document ownership. Read tracked .superpowers/sdd/status.md for the portable resume checkpoint and the active plan ledger if it exists locally; fresh clones also use the permanent plan and validation report. Update local progress after verified work.
- Product scope, architecture, safeguards, decisions, UX and roadmap each have an owning HTML document linked from the index.
- Follow docs/engineering/engineering.html#documentation-layout for documentation placement. Permanent specs, plans, phase validation reports and narrowly curated proof belong under docs/superpowers/. Execution logs, detailed progress, diffs, checks, handoffs, working notes and temporary Markdown belong in ignored .superpowers/sdd/. The concise portable status.md checkpoint is the only tracked exception. Archive generated operational outputs there after runs.
- Keep requirements in their owning document and link to them; avoid duplicating instructions across files.
- Preserve the approved UI defined in [UX workflows](docs/ux/workflows.html), its linked approved specifications and [current visual references](docs/ux/mocks/index.html). Reuse existing colour tokens, surface hierarchy, layout, navigation and interaction patterns when extending features. Do not redesign or replace these conventions unless the user explicitly requests a UI change. For requested changes, update the owning UX documentation and relevant specs/mocks alongside implementation; historical or illustrative mocks do not override the current approved design.
- Planning approval is not implementation authorization. Use Superpowers planning/spec workflows and begin code only when the user requests implementation.
- Do not claim Lens/K9s parity without reviewed capability coverage and evidence.

## Engineering and security
- For Kubernetes readers, relationships, status and integration tests, use the [KubeSift Kubernetes development skill](.agents/skills/kubesift-kubernetes-development/SKILL.md) and its linked reference index.
- Follow the selected architecture and docs/security/security-model.html. Keep domain services independent of the desktop bridge.
- Use explicit connection/session/resource targets; never execute against a mutable global current context.
- Apply cancellation, flow control and bounded buffering to long-lived sessions; verify cleanup.
- Use disposable local clusters for development and mutation tests. Identify explicit external contexts/namespaces and allowed actions before integration tests; never test permissions with trial mutations.
- App review and agent development use only the kind contexts in docs/engineering/local-development.html. Create them with scripts/dev-clusters.sh. Do not create extra clusters. Agents may perform the actions the work requires inside dev-local-a and dev-local-b, and must pass an explicit context.
- Do not add infrastructure to clusters for the shipped application's functionality.
- Use synthetic test data; follow the security model for credential handling.
- Run meaningful checks appropriate to each change and distinguish completed validation from unrun checks.
- Follow [agent verification and acceptance](docs/engineering/engineering.html#agent-verification): the implementing agent owns automated and desktop computer-use checks; do not require a duplicate user UAT checklist by default.
- Follow the project layout and plain-English source description contract in docs/engineering/engineering.html#source-layout.
- Follow docs/engineering/engineering.html#authoring-style when writing and formatting changes.
- Engineering practice lives in docs/engineering/engineering.html. Product scope, architecture, safeguards, and phase order stay in their owning documents.

## Git and publication
- Follow docs/engineering/gitworkflow.md.
- Apply its [independent PR review gate](docs/engineering/gitworkflow.md#independent-pr-review) once to the complete implementation using a fresh reviewer and the repository code-review skill. No routine per-task reviews; fixes receive targeted follow-up before merge.
- Complete authorized implementation through the workflow's documentation, ready PR and merge steps into develop once applicable checks and independent review pass. Do not stop at a draft PR or request another instruction to merge; report genuine blockers instead. Releases and merges into main remain separately authorized.
