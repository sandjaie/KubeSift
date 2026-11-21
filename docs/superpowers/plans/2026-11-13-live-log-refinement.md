# Live log refinement implementation plan

> Execute with the Superpowers executing-plans workflow in this session.

Approved by the user on 2026-11-13. Extend Phase 1C.2 task 7 on its existing feature branch.

Goal: one automatically opened live log view, cancelled on departure and fresh on return, with scrolling, search and expansion inside the application window.

Architecture: retain the existing native stream and bounded log store. PodLogs owns an abortable request for its visible lifetime. LogViewer renders the retained 1 MiB as bounded text, with search selecting one match. Remove the product Sessions dock because these logs have no background lifetime.

- [x] Add an abort lifecycle regression covering departure during pending open and late native completion.
- [x] Wire automatic current-container follow, previous snapshots, replacement on valid filter changes and Refresh into PodLogs.
- [x] Replace segmented presentation with a scrollable retained text view, previous/next search matches, Jump to latest, and window expansion with Escape restoration.
- [x] Update owning UX/spec, source descriptions and UAT instructions. Preserve explicit target validation and loss warnings.
- [x] Run frontend and repository checks, package, verify the native local-cluster journey and review the diff. Commit and push to the existing branch.

Review rapid navigation, pending opens, invalid line input, previous-container logs and search while output grows. History remains bounded at 1 MiB and the requested initial line count, with visible omissions. User acceptance remains separate.
