# KubeSift design specification

Date: 2026-11-05
Status: draft for review; not implementation authorization

## Intent

Build a local-first desktop workspace for DevOps/SRE engineers with broad Kubernetes operations in Phase 1 and evidence-backed AI investigation in Phase 2. The first user works with company OpenShift/EKS/AKS clusters; personal clusters and personal AI providers must also have full product access subject to permissions and available services.

## Authoritative design package

This specification connects the HTML documents rather than duplicating their requirements:

- [Product requirements](../../product/requirements.html): scope, non-goals and acceptance journeys.
- [Capability inventory](../../product/feature-matrix.html): coverage and unresolved feature parity.
- [Architecture](../../architecture/overview.html): components, identity, authentication, streaming, metrics and packaging.
- [Security model](../../security/security-model.html): safeguards, credential-helper trust, operations and AI evidence controls.
- [Decisions](../../architecture/decisions.html): accepted choices, proposed defaults and unresolved decisions.
- [UX workflows](../../ux/workflows.html): navigation, session behavior and error states.
- [Roadmap](../../roadmap/roadmap.html): milestone order, dependencies and exit criteria.

If these sources conflict, resolve the conflict in the owning document before implementation. The HTML index is the human-readable entry point.

## Approaches considered

The desktop choice is Wails + Go + React/TypeScript. Tauri 2 + Go was considered for its explicit native capabilities and separate backend process. It introduces Rust integration and sidecar lifecycle/protocol/packaging work. The user chose Wails with a required early validation gate. The choice does not establish measured streaming performance or packaged authentication compatibility.

The product approach is a broad operations client before AI, rather than a narrowly scoped AI troubleshooting MVP. Delivery is incremental, with the roadmap owning the subdivision.

## Constraints and anti-goals

Follow the authoritative product and security requirements. In particular, avoid wrong-cluster operations, hidden credential-helper execution, silent external-provider fallback, unbounded streams, undocumented scope exclusions and claims of parity without evidence. The app must not install infrastructure for its own functionality. A public codebase must not contain company material.

## What is left to the builder

After design review and implementation authorization, choose exact dependencies, package layout, local cluster tooling and adapter details within these boundaries. Set performance budgets from validation measurements. Do not silently decide unresolved product features, provider data policies, license or release platform coverage.

## Review and planning gates

1. Review this specification together with its linked HTML documents.
2. Resolve significant design questions or record explicitly deferred choices in the decision register.
3. Obtain approval of the written design.
4. Use the Superpowers writing-plans skill to create detailed, independently verifiable execution tasks under `docs/superpowers/plans/`.
5. Execute only when the user separately requests implementation, starting with the validation milestone.

The existing Phase 1A outline records the validation experiment requested during discussion. It is not a test-driven execution plan and does not authorize application scaffolding.
