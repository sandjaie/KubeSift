# Workspace refinement proof

Native packaged-app captures from 12 November 2026, using only existing local kind fixtures. See the [validation report](../../validation/2026-11-12-workspace-refinement.html) for methods, artifact provenance and coverage limits.

- `dark-wide.png`: neutral dark, side-by-side list/details, local A selected.
- `dark-stacked.png`: independent stacked detail share with the unavailable AI panel on the right.
- `namespace-search.png`: search inside the namespace dropdown; typing does not change the active target.
- `light-wide.png`: preserved Light palette with the updated selector and divider.

These are observations of the implemented UI, not synthetic preview modes. No credential bodies or external context details are present. The generated mock lives separately under docs/ux/mocks/.

Compact follow-up (corrected final package):

- `compact-dark.png`: two-row header, inline Refresh, phase dots and compact details.
- `compact-connection.png`: explicit connection source and reconnect/disconnect menu.
- `compact-stacked.png`: scrolled Pod rows with sticky headings and AI unavailable pane.

The four earlier captures above document the initial refinement, before the compact follow-up.
