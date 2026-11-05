# KubeSift repository instructions

## Planning and scope
- Read progress.md for current task status; keep it short and update it after verified progress. Do not put the shared progress.md in per-task scratch folders.
- Start at docs/index.html. Product scope, architecture, safeguards, decisions, UX and roadmap each have an owning HTML document.
- Markdown Superpowers specifications live in docs/superpowers/specs/; detailed execution plans belong in docs/superpowers/plans/.
- Keep requirements in their owning document and link to them; avoid duplicating instructions across files.
- Planning approval is not implementation authorization. Use Superpowers planning/spec workflows and begin code only when the user requests implementation.
- Do not claim Lens/K9s parity without reviewed capability coverage and evidence.

## Engineering and security
- Follow the selected architecture and docs/security/security-model.html. Keep domain services independent of the desktop bridge.
- Use explicit connection/session/resource targets; never execute against a mutable global current context.
- Apply cancellation, flow control and bounded buffering to long-lived sessions; verify cleanup.
- Use disposable local clusters for development and mutation tests. Identify explicit external contexts/namespaces and allowed actions before integration tests; never test permissions with trial mutations.
- Do not add infrastructure to clusters for the shipped application's functionality.
- Use synthetic test data; follow the security model for credential handling.
- Run meaningful checks appropriate to each change and distinguish completed validation from unrun checks.
- Follow the project layout and plain-English source description contract in docs/engineering/engineering.html#source-layout.
- Engineering practice lives in docs/engineering/engineering.html. Product scope, architecture, safeguards, and phase order stay in their owning documents.

## Git and publication
- Follow docs/engineering/gitworkflow.md.
