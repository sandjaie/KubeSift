# Neutral dark palette and workspace controls

Approved direction: user chose the readability-adjusted neutral palette, searchable namespace selector, and remembered draggable Pod-details divider, then approved the illustrative mock and explicitly requested implementation on 12 November 2026. Work stays on `feature/workspace-theme-and-controls`, created from develop; no worktrees.

## Intent and scope

Keep the existing workspace structure and resource capabilities. Replace the blue-green dark surfaces with layered neutral greys; simplify namespace selection; let people allocate space between the Pod list and details. This is UI refinement following Phase 1C.1, not Phase 1C.2, AI integration, or a release.

## Approaches

1. Exact supplied palette: closest literal colour match, but some small text/button combinations have insufficient contrast.
2. Supplied palette with readable text adjustments: selected by the user. Preserve the neutral surfaces and semantic colour families, adjusting foregrounds where necessary.

Reuse the existing React presentation, native preference store and accessible separator. Add no runtime dependencies. The existing Light appearance remains visually unchanged except for the explicitly requested controls. Historical validation remains historical; updated mocks are illustrative references, not evidence of new resource capabilities.

## Visual contract

Dark and System-dark use base `#1A1D21`, panel `#22252A`, hover/selection `#2C2D30`, overlay `#35373B`, borders `#383A40`, primary text `#D1D2D3`, headings `#E8E8E8`, secondary text `#A3A5A8`, blue interactions `#1D9BD1`, green success `#2EB67D`, amber warning `#ECB22E`. Use lighter blue `#6FC8F0` for interactive foreground text on dark surfaces. Use dark text on blue buttons and light red `#FF839F` for error text. Faint disabled grey is never used for required metadata. All surfaces are flat; shading and invented row counts in the generated mock are not implementation requirements. Selected controls retain an indicator as well as colour. Floating dialogs and namespace options use the overlay level. Success is independent of the interaction accent.

## Namespace selector

One button-like control shows the active namespace and opens a popover containing a search input and filtered namespace options. This follows the approved mock: search lives inside the dropdown, not beside it. Click or Enter commits a selection; typing alone never requests data. Escape and outside focus close without changing the active namespace. Keyboard arrows navigate options and selected state is announced. Keep explicit tab/namespace request ownership.

When discovery is incomplete or restricted, a syntactically valid unlisted namespace offers `Use “name”`. Complete lists do not invent matches. Always include the current explicit namespace, even when it is absent from discovery. No Go button or duplicate Workloads / Pods text in the identity bar; the existing resource toolbar breadcrumb remains.

## Pod detail divider and persistence

The border between list and details becomes a pointer/keyboard separator with visible grip, resize cursor and hover/focus indication. Vertical in side-by-side layout; horizontal in stacked layout. Remember the detail share separately for both orientations, globally like the existing pane dimensions. Defaults are 50%; stored values are 20–80%. Effective display clamping preserves usable list/detail space and never overwrites a larger-screen preference merely because the window shrank. Keep existing responsive layout choices.

Migrate valid version-1 preferences to version 2 in memory, preserving appearance, window and previous pane dimensions. New saves persist both percentages; old files are not rewritten merely by loading. Reject null, unknown, duplicate, malformed and out-of-range settings as before. The new fields are required in version 2. No resource data or connection identities are persisted.

## Validation and documentation

Focused tests: legacy preference migration and split round trip/rejected dimensions; namespace filtering/current value/unlisted permissions. Native packaged checks: neutral dark and unchanged light, dropdown keyboard/click/dismiss/manual handling, both divider orientations and restart persistence, A/B target isolation. Only kind-dev-local-a/b with explicit targets; no new cluster fixtures required. Full repository gate and package build, fresh correctness/hygiene review, actual results and limitations in a permanent report. Update UX owner, mock gallery, index and tracked portable status; detailed logs remain ignored. Commit and push the feature branch and open a review PR; do not merge without a new instruction.

## Approved compact workspace follow-up · 12 November 2026

The maintainer approved the second illustrative mock and requested implementation in the same branch and PR. Replace the four-row header stack with two rows: global app navigation, then the resource-navigation toggle, explicit cluster, namespace selector and connection-status disclosure. Keep namespace search and independent saved dividers unchanged. The status disclosure contains the full source path, context, last successful read and Reconnect/Disconnect actions. Reconnect uses existing Retry credential reload semantics; Disconnect retains existing cancellation/tab-close semantics, explained in the disclosure. Connecting exposes Cancel; failed/stale states expose Retry without needing the menu, and helper review remains accessible. Errors stay visible only when present. No new service calls or weakened target safeguards.

Remove the separate resource toolbar. Put Refresh in the Pods heading and keep its accessible name Refresh Pods; unavailable kinds retain a heading and navigation toggle. Sticky table column headings remain inside the scrolling list. Remove repeated namespace subtitles from single-namespace rows. Phase dots accompany text: Running blue, Succeeded green, Pending amber, Failed red and Unknown grey. Phase does not imply readiness; retain the separate readiness column. Detail uses compact label/value rows, context/namespace visible and an expandable Identity & source block for UID and full source path; these values remain selectable for copying. No clipboard integration is required.

Alternative considered: compress the old four bars. This saves less room and retains duplicate controls; the approved mock instead moves contextual actions beside their owning information. Flat production colours remain authoritative over generated shading. Keep errors, long identities, keyboard focus and dropdown dismissal usable. This is the approved UI refinement, not additional resource functionality or Phase 1C.2.

Light phase colours use a dedicated Running blue (#1264a3) and retain Succeeded green (#007f72). Dark Running remains #6fc8f0. Historical mock and screenshot changes are summarized in text; retain only the current approved mock and compact workspace evidence in the gallery.
