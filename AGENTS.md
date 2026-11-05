# KubeSift repository instructions

## Planning and scope
- Start at docs/index.html. Product scope, architecture, safeguards, decisions, UX and roadmap each have an owning HTML document.
- Markdown Superpowers specifications live in docs/superpowers/specs/; detailed execution plans belong in docs/superpowers/plans/.
- Keep requirements in their owning document and link to them; avoid duplicating instructions across files.
- Planning approval is not implementation authorization. Use Superpowers planning/spec workflows and begin code only when the user requests implementation.
- Do not claim Lens/K9s parity without reviewed capability coverage and evidence.

## Engineering and security
- Follow the selected architecture and docs/security/security-model.html. Keep domain services independent of the desktop bridge.
- Use explicit connection/session/resource targets; never execute against a mutable global current context.
- Apply cancellation, flow control and bounded buffering to long-lived sessions; verify cleanup.
- Use disposable local clusters for development and mutation tests. Identify exact company contexts/namespaces and allowed actions before integration tests; never test permissions with trial mutations.
- Do not add infrastructure to clusters for the shipped application's functionality.
- Use synthetic test data. Keep credentials, kubeconfigs, company resource data, internal endpoints, logs and screenshots out of the repository.
- Run meaningful checks appropriate to each change and distinguish completed validation from unrun checks.

## Git and publication
- Use the user's personal GitHub identity. Before commits, verify repository-local author identity; before remote operations, verify remote destination and authentication account separately.
- Do not infer the personal account from the OS username, use a company account, or change global Git identity. Ask if the intended personal account is ambiguous.
- Do not push or publish without authorization. License is deliberately undecided; do not add a license by assumption.
