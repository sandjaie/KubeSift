# Phase 1B UI Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task in the separate implementation session. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver the approved, interactive desktop workspace with explicit synthetic previews, preserving the validated live interface until Phase 1C connects the full explorer.

**Architecture:** Separate the product shell and preview presentation state from the existing validation controller and session bridge. Keep preview data entirely in memory; a small Go settings service owns validated appearance and geometry persistence. Preserve the existing live validation interface behind an explicitly labeled entry point rather than rebuilding Kubernetes services in this phase.

**Tech Stack:** Existing pinned Wails v3, Go, React, TypeScript, CSS, Vitest and xterm. No framework upgrade, AI SDK, component framework, routing library or state-management dependency is required. Use native semantic controls and small local components; revisit a dependency only if a concrete accessibility requirement cannot reasonably be met.

**Spec:** [Approved Phase 1B UI specification](../specs/2026-11-06-phase-1b-ui-design.md), approved 2026-11-06.

**Execution status:** Not started. This is a plan for a separate session. Phase 1A has reviewed local acceptance with named follow-ups; see [actual validation](../validation/phase-1a.html). Planning was prepared from `develop` after Phase 1A merge; recheck the implementation checkout before applying file moves.

## Global constraints

- Follow [engineering practices](../../engineering/engineering.html), [Git workflow](../../engineering/gitworkflow.md), [UX behavior](../../ux/workflows.html) and [security](../../security/security-model.html). These remain the owners of permanent rules.
- “System default with persistent Light/Dark overrides, changed only in Settings.”
- “Demo operation controls never invoke live mutation/session tools.”
- “Do not mix fixture rows into a live cluster list or silently substitute demo data on errors.”
- “Do not introduce real API-key setup, model SDKs or outbound AI requests in 1B.”
- “Proposed validation sizes: 1440×900 and 1920×1080, plus a candidate minimum 1100×700 logical window size.” Measure before accepting the minimum; preserve legible controls and scrolling.
- Every new/changed Go, TS and TSX file needs the prescribed Purpose / Used by / Responsibilities / Important description. Generated bindings come from the binding task, never handwritten edits.
- Keep operational logs, progress, reviews and handoff notes in ignored `.superpowers/sdd/2026-11-06-phase-1b-ui-foundation/`. Permanent plans/specs/reports belong under `docs/superpowers/`. Do not put progress or plans in the root README.
- No cluster setup or mutation is required to prove preview UI. Any live regression uses the existing approved local targets and ownership rules; do not create extra routine-development clusters.

## Review focus

1. Same-named resources in two tabs and a review left open during navigation: target stays fixed. Automated state check in Task 2; manual review in Task 6.
2. Invalid settings or disconnected displays: reject malformed preferences and keep a usable on-screen window. Small Go settings/geometry check in Task 4; native monitor check in Task 6.
3. Typing in an editor, dialog or terminal: global shortcuts must not steal input. Keyboard manual check in Task 3/6.
4. Hiding a dock or changing theme while a live terminal runs: do not unmount/recreate or replay its session. Existing routing checks plus a focused live manual check in Task 6.
5. Switching page/tab while AI preview is open: preserve investigation identity and keep the current page accessible. State assertion in Task 5 and manual check in Task 6.

## Scope map: live versus preview

| Surface | Phase 1B behavior |
| --- | --- |
| Normal startup | Product shell, empty welcome page; no context auto-open and no preview silently selected |
| UI preview | Explicit “Explore UI preview” entry; persistent “UI preview · synthetic data” banner; only synthetic tabs/resources/sessions/actions |
| Live validation | Explicit “Local validation workspace” entry retains 1A connection/helper/Pod/session/fixture controls; clearly labeled, not presented as the finished explorer |
| Settings | Real theme and pane/window preferences; helper trust remains in the existing live validation interface, linked from Settings |
| Resource pages | Synthetic Pod and generic-resource previews; unavailable future kinds explain that live browsing arrives in the explorer milestone |
| Review dialogs | Simulated operation review only in preview; existing live fixture write remains only in validation mode and retains its backend safeguards |
| AI | Working right-panel layout in preview, synthetic evidence/payloads; normal mode explains availability without collecting data or contacting a provider |

Mode choice is session-local, never persisted. Product/preview components may use settings APIs, but must not import generated Kubernetes/session/operation service methods. Keep live validation mounted once opened, hide it when navigating away, and offer an explicit close-all action using its existing close protocol. Do not treat hiding as backend cleanup. Preview sessions never construct a real `SessionRef` or invoke the live `Terminal` component.

## File map and contracts

Existing files inspected: `frontend/src/App.tsx` is the compact live validation controller; `state.ts` routes explicit sessions and bounds history; `bridge.ts` owns events/activation/acks; `Terminal.tsx` owns xterm subscription/input/resize; `profile.ts` and `validation.ts` support packaged validation. `internal/desktop/app.go` creates a 1100×760 window; no general UI settings service exists.

| Files | Responsibility |
| --- | --- |
| `frontend/src/ValidationApp.tsx` (move existing App), `frontend/src/validation.css` (move existing styles) | Preserve 1A controller and scope its CSS beneath its host |
| `frontend/src/App.tsx`, `frontend/src/styles.css` | New application entry/shell composition and shared token/layout styles |
| `frontend/src/ui/Controls.tsx`, `frontend/src/ui/ReviewDialog.tsx` | Small shared status/empty-state controls and accessible frozen-target dialog |
| `frontend/src/workspace/model.ts`, `workspace/state.ts`, `workspace/state.test.ts` | Presentation types, reducer and narrow behavioral tests |
| `frontend/src/workspace/Workspace.tsx`, `ClusterTabs.tsx`, `ResourceNavigation.tsx`, `ResourceTable.tsx`, `ResourceDetail.tsx` | Shell, navigation, list and detail rendering |
| `frontend/src/preview/fixtures.ts` | Consistent synthetic data and explicit preview factory |
| `frontend/src/sessions/SessionsDock.tsx` | Synthetic session dock; no native session side effects |
| `frontend/src/workspace/commands.ts`, `CommandPalette.tsx` | One command registry and focus-aware shortcut dispatch |
| `frontend/src/settings/SettingsPage.tsx`, `preferences.ts` | Appearance/pane preferences and generated settings-service adapter |
| `frontend/src/ai/InvestigationPanel.tsx` | Synthetic conversation, evidence and payload preview |
| `internal/preferences/store.go`, `store_test.go` | Strict versioned preferences persistence, defaults and pure rectangle clamping |
| `internal/desktop/preferences.go`, `app.go` | Thin settings RPC and native window restoration/save lifecycle |
| `docs/superpowers/validation/phase-1b.html` | Final actual UI acceptance report, created at completion |

Use these paths as the initial decomposition, not a reason to create empty modules. Do not split trivial wrappers into more files. Update existing folder README descriptions only when affected by an actual move.

### Shared presentation types

Define these in `workspace/model.ts`; they do not replace `bridge.ts` identities:

- `WorkspaceMode = 'welcome' | 'preview' | 'validation'`.
- `NamespaceScope = {kind:'all'} | {kind:'namespace'; name:string} | {kind:'cluster'}`; never use an ambiguous empty string to mean all three.
- `ResourceKey = {clusterId:string; apiVersion:string; kind:string; namespace:string|null; name:string; uid:string}`. Preview IDs have a `preview:` prefix and are never valid backend identities.
- `ResourceView = {key:ResourceKey; status:string; ready:string; containers:string[]; summary:Record<string,string>; yaml:string; events:{id:string; at:string; reason:string; message:string}[]; logs:{id:string; at:string; severity:'info'|'error'; text:string; container:string; previous:boolean}[]}`.
- `ViewStatus = 'ready' | 'loading' | 'empty' | 'forbidden' | 'disconnected' | 'stale' | 'unsupported' | 'missing-dependency'`.
- `TabView = {id:string; label:string; environment:string; scope:NamespaceScope; resourceKind:string; filter:string; selected:ResourceKey|null; detail:'overview'|'events'|'logs'|'yaml'|'terminal'; scroll:{list:number; detail:number}; status:ViewStatus}`.
- `PreviewSession = {id:string; kind:'logs'|'terminal'|'forward'; target:ResourceKey; container?:string; status:'running'|'stopped'; text:string}`.
- `ActionReview = {id:string; action:'restart'|'delete'; target:ResourceKey; description:string}`. Freeze a copy, invalidate if its owning preview resource is removed/replaced, never resolve it from active selection on confirm.
- `InvestigationView = {id:string; target:ResourceKey|null; collectedAt:string; evidenceIds:string[]}`.
- `UiState = {mode:WorkspaceMode; page:'resources'|'settings'; activeTab:string|null; tabs:TabView[]; sessions:PreviewSession[]; review:ActionReview|null; dockOpen:boolean; aiOpen:boolean; investigation:InvestigationView|null}`.

The single `UiAction` discriminated union covers entering a mode; opening/selecting/updating/closing a preview tab; updating page/scope/filter/selection/detail/scroll/status/environment; preparing/confirming/dismissing a review; starting/stopping a preview session; toggling dock/AI; and explicitly starting/rebinding an investigation. Each targeted action carries tab ID or a copied resource key. `reduceUi(state:UiState, action:UiAction):UiState` rejects unknown owners and stale resource identity; `initialUiState():UiState` returns the welcome state. Keep preview data separate from preferences and from the live event reducer.

## Task 1: Establish the product entry and shared visual foundation

**Files:** Move `frontend/src/App.tsx` → `ValidationApp.tsx`, `styles.css` → `validation.css`; create the new `App.tsx`, `styles.css`, `ui/Controls.tsx`; adjust `main.tsx` only if necessary. Update moved-file descriptions.

**Interfaces:** `ValidationApp(): JSX.Element` preserves existing behavior. `StatusBadge({label,tone})` uses `tone:'neutral'|'success'|'warning'|'danger'`; `StateMessage({status,title,detail})` consumes `ViewStatus` from Task 2 (define this small type now in `workspace/model.ts`). Root App owns mode selection and lazy mounting of the live validation view.

- [ ] Read current spec, engineering instructions, 1A report and current diff; work on a `feature/*` implementation branch based on the reviewed planning commit. Confirm toolchain readiness without installing or upgrading dependencies opportunistically.
- [ ] Move the existing validation component mechanically; preserve service calls, guards, profile autorun, session registration and counters. Scope its CSS to `.validation-workspace` so its generic selectors do not style product controls. Keep the existing validation state/bridge/terminal modules in place.
- [ ] Build the welcome entry and semantic color/spacing/type tokens using the approved mocks. Shared controls include focus-visible styles and textual status cues. Add explicit preview and validation entry buttons; normal startup opens neither automatically.
- [ ] Preserve the explicit validation-profile launch path: App may inspect `Service.ValidationConfiguration()` after Ping; if its guarded `autorun` is true, select the validation view, whose existing controller still owns profile execution. Do not duplicate `runProfile`, bypass the backend fixture guard, or cause two profile starts. Keep the ready notification behavior working in normal startup too.
- [ ] Run `npm test`, `npm run lint`, `npm run typecheck`, `npm run build`; existing routing test passes with no changes to its assertions. Manually check welcome/validation entry and plain-text rendering of hostile strings. No new automated test for static token markup.
- [ ] Commit `refactor: separate validation workspace from product shell` with only task files.

## Task 2: Build isolated cluster tabs and the explorer layout

**Files:** `workspace/model.ts`, `state.ts`, `state.test.ts`, `Workspace.tsx`, `ClusterTabs.tsx`, `ResourceNavigation.tsx`, `ResourceTable.tsx`, `preview/fixtures.ts`; modify `App.tsx`, `styles.css`.

**Interfaces:** shared types above; `createPreview(): {tabs:TabView[]; resources:ResourceView[]}`; `Workspace({state,dispatch,resources})`; child components consume the selected `TabView`, relevant resource rows and explicit callbacks. No generated service imports in these files.

- [ ] Write a failing `keeps tab navigation and review targets isolated` Vitest case in `workspace/state.test.ts`: select same-named Pods in preview A/B, set different namespace/filter/detail/scroll, prepare a review on A, switch to B, assert A’s fields and review target unchanged; close/remove A and assert confirmation no longer succeeds. Run `npm exec --workspace frontend -- vitest run src/workspace/state.test.ts` and observe the failure.
- [ ] Implement the model, reducer and factory. Include two synthetic clusters with same-named Pods but distinct UIDs/namespaces, a missing-DATABASE_URL Pod with consistent log/event/config evidence, a healthy Pod and one generic custom resource. Display stable fixture timestamps as synthetic capture times, never as live collection.
- [ ] Implement tabs, explicit identity bar, environment-label editing, namespace scope, grouped navigation, resource selection and filtering. Navigation entries share one registry for label/kind/availability. Unimplemented destinations render an honest unavailable view, not fabricated live data.
- [ ] Restore per-tab scroll when switching, retain selection by full key, and clear selection when scope makes it ineligible. All-namespaces/cluster views must not produce a namespaced action from an ambiguous target. Render the preview banner throughout preview pages.
- [ ] Run the targeted state test, lint and typecheck; manually switch A/B and confirm independent state. Commit `feat: add preview workspace with isolated cluster tabs`.

## Task 3: Add detail views, simulated actions, sessions and keyboard controls

**Files:** `workspace/ResourceDetail.tsx`, `sessions/SessionsDock.tsx`, `ui/ReviewDialog.tsx`, `workspace/commands.ts`, `CommandPalette.tsx`; modify `state.ts`, `state.test.ts`, `Workspace.tsx`, `styles.css`.

**Interfaces:** `ResourceDetail({resource,detail,onDetailChange,onAction,onStartSession})`; `SessionsDock({sessions,open,onToggle,onStop})`; `ReviewDialog({review,onConfirm,onCancel})`. `CommandId = 'palette'|'logs'|'terminal'|'forward'`; one registry owns labels, availability and shortcut text. `commandForKey(event:KeyboardEvent):CommandId|null` returns no global command for editable targets, terminals, open modal inputs or IME composition.

- [ ] Extend the existing new state case before implementation: opening/stopping a preview session acts only on its captured owner; collapse and tab switch leave it present; cancel tab-close leaves sessions untouched, confirm closes only that tab’s sessions. Observe targeted failure before fixing behavior.
- [ ] Build Overview/Events/Logs/YAML and a terminal preview panel. YAML is read-only presentation in 1B. Logs have container/previous selection and an explicit follow preview; previous disables follow. Render untrusted text without HTML injection, with textual severity and error gutter. Generic details use the same identity and YAML pattern.
- [ ] Build simulated restart/delete review. Use an accessible dialog with focus containment, Escape/cancel, labeled controls and focus return; confirm is never the default Enter action on opening. Show copied full target and “Simulate” wording; stale review requests a fresh review.
- [ ] Build preview Sessions dock with stable keys and explicit stop buttons. Simulated terminal output is inert text, never the native `Terminal` component. Keep sessions when changing resource pages/settings, themes or dock visibility. A tab with sessions asks before removal.
- [ ] Implement command palette and shortcuts: Cmd/Ctrl+K palette, Cmd/Ctrl+Shift+L logs, Cmd/Ctrl+Shift+T terminal preview, Cmd/Ctrl+Shift+P forward preview. Table arrows move selection; no resource means actions disabled. Commands call the same callbacks as buttons with a copied target. Scope shortcuts to the product workspace; do not register them over the live validation workspace.
- [ ] Run targeted state test, lint/typecheck; manually check keyboard selection, typing in filter, read-only YAML selection, modal focus/Escape and simulated destructive review after tab switch. Commit `feat: add resource details and session interaction previews`.

## Task 4: Persist appearance and make the workspace adaptive

**Files:** `internal/preferences/store.go`, `store_test.go`, `internal/desktop/preferences.go`, `app.go`; `frontend/src/settings/preferences.ts`, `SettingsPage.tsx`; modify `App.tsx`, `Workspace.tsx`, `styles.css`. Regenerate bindings using the existing task.

**Interfaces:** Go `Rect{X,Y,Width,Height int}`, `Preferences{Version int; Appearance string; Window *Rect; NavigationWidth, AIWidth, DockHeight int}` with lower-camel JSON fields. `Default() Preferences`, `Validate(Preferences) error`, `ClampWindow(saved Rect, workAreas []Rect) Rect`, `Load(path string)(Preferences,error)`, `Save(path string, prefs Preferences) error`. Desktop `GetPreferences()(preferences.Preferences,error)`, `SetPreferences(prefs preferences.Preferences) error`; these two methods belong to a separately registered `PreferencesService`, not the Kubernetes Service. UI adapter `loadPreferences():Promise<Preferences>`, `savePreferences(value:Preferences):Promise<void>` uses generated types.

- [ ] Write two small Go tests before code: strict settings round trip rejects unknown keys/unsupported version/invalid appearance and preserves existing file on rejected save; window clamping handles an off-screen saved rectangle and a smaller current work area, producing positive bounds entirely within it. Use temp directories and synthetic rectangles, no real settings. Run `go test ./internal/preferences` and observe failure.
- [ ] Implement version 1 settings. Defaults: appearance `system`, no saved window, navigation width 220, AI width 360, dock height 220 logical pixels. Validate appearance against `system|light|dark`; reject unknown keys, trailing JSON, unsupported version and invalid dimensions. Keep negative X/Y legal for displays left/above primary. Accept finite bounded integer pane sizes; define ranges navigation 160–320, AI 300–560, dock 120–480. A loaded invalid file yields visible warning plus in-memory defaults, never silent overwrite; offer explicit reset. Missing file uses defaults.
- [ ] Store atomically at `os.UserConfigDir()/kubesift/preferences.json` with restrictive file permissions and a bounded read (64 KiB). The public RPC never accepts an arbitrary path. Serialize updates so native geometry saving does not overwrite a newer appearance choice. No credentials, context identities, resource caches, session reconnect state or preview conversations go in this file.
- [ ] Wire the thin preferences service and native window lifecycle in `app.go`. Use the pinned Wails API after inspecting its local module/current official documentation; do not guess API names. Restore into current display work areas, clamp on display changes and save only normal (not minimized/fullscreen) bounds after a debounce. Prefer maximum-overlap display or primary when none; if available area is smaller than the candidate minimum, fit the actual area and enable scrolling. Startup fallback is a centered usable window. Keep Wails imports out of `internal/preferences`.
- [ ] Implement Settings and apply theme tokens using `matchMedia('(prefers-color-scheme: dark)')` for System, reacting to OS changes and cleaning listeners. Failed saves report a useful error rather than claiming persistence. Retain tab/session state across theme changes. Display Settings link to helper trust in the retained validation workspace; no duplicate trust store.
- [ ] Add resizable navigation/AI/dock separators with keyboard adjustment and labels. Clamp effective widths to available space without permanently overwriting a user’s preferred wide-monitor sizes. At narrow sizes stack list/detail in the main area, keep AI on the right, permit user-controlled nav/dock collapse and pane scrolling. Restore geometry without hiding target/action controls. Update xterm theme in-place only if adapting the retained validation theme; never put theme in its mount/session-effect dependencies.
- [ ] Run `go test ./internal/preferences`, `go test ./...`, `go vet ./...`, bindings generation, frontend tests/lint/typecheck/build. Manually exercise System/Light/Dark, OS appearance change and packaged restart. Commit `feat: persist appearance and adaptive workspace geometry`.

## Task 5: Build the global AI design preview

**Files:** `frontend/src/ai/InvestigationPanel.tsx`; modify `workspace/state.ts`, `state.test.ts`, `Workspace.tsx`, `App.tsx`, `preview/fixtures.ts`, `styles.css`.

**Interfaces:** `InvestigationPanel({investigation,resources,onStartNew,onClose})` consumes `InvestigationView` and synthetic `ResourceView[]`. The existing reducer owns open/close and explicit rebind; the component owns draft text only while mounted. Keep it mounted when closed to preserve preview conversation during the session.

- [ ] Add a focused assertion to the existing state test: open an investigation on A, navigate to B and Settings, close/reopen the panel, and assert A identity and evidence IDs remain; only explicit start-new/rebind may change target. Null-target investigation stays null until an explicit selection. Observe the targeted failure first.
- [ ] Put Ask AI in the consistent shell action area on all product pages, including welcome, Settings and validation entry. In preview open a resizable right panel; in normal/live mode show its availability explanation in that same panel with no collection or network action. Do not place a modal or cover the current page.
- [ ] Show synthetic provider/model, target/no-target, observations/hypotheses/unknowns and timestamped evidence links. On settings with no selected resource, ask for a preview target before generating a synthetic response. Open citations against the captured fixture identity, not the currently selected row.
- [ ] Implement “View redacted payload” and “View sent payload” presentations using fixed synthetic data, clearly marked “Example only — nothing sent”. Sample response is triggered explicitly, not presented as live model output. No real redaction pipeline, key fields, outbound request or generic tool executor.
- [ ] Run state test, lint/typecheck/build; manually open/close from resource/settings/no-target pages at laptop width and verify list accessibility and stable target. Commit `feat: add contextual AI investigation preview`.

## Task 6: Verify the packaged UI and prepare the next-phase handoff

**Files:** modify relevant UI only to fix observed issues; create `docs/superpowers/validation/phase-1b.html`; update `docs/index.html`, roadmap/UX links only to reflect actual reviewed results. Curated screenshots, if needed, go under `docs/superpowers/evidence/phase-1b/` with a small manifest; do not create another mock directory.

**Interfaces:** consumes the complete shell and unchanged live service contracts. Produces an HTML validation report and concise ignored implementation handoff.

- [ ] Run `scripts/check.sh` and `wails3 task package` using the recorded toolchain. Launch the resulting `bin/kubesift.app` through Finder. Do not report a browser preview as native validation. Record failures/unrun checks distinctly.
- [ ] Run the focused manual matrix below. Capture a few useful synthetic screenshots: wide/dark with AI, laptop/light with dock and minimum-size stress view. Check both themes at all three sizes without creating six permanent near-duplicate images. Record actual logical dimensions and host/scale.
- [ ] If live validation UI was moved or its wrapper changed, verify against an approved existing local target: open one log and terminal session, switch to product Settings/preview and back, collapse/restore, change theme and explicitly close. Assert no remount/replayed command, correct target, visible stream loss/error and released session/forward on stop. Use only a benign terminal command on the existing fixture. Do not run trial mutations, create extra clusters or rerun the 600-second 1A load for a presentation-only change. If backend session/flow-control code changed unexpectedly, stop and reassess scope/checks.
- [ ] Review the diff for correctness and then code hygiene per engineering practice; preserve the known 1A backgrounded WebKit log-loss limitation. This UI milestone does not claim to fix it or validate sleep/wake/cloud auth/signing.
- [ ] Write the report with each spec acceptance item, outcome/evidence, live/preview boundary, final minimum-size decision and remaining limitations. Update documentation links and local SDD status after verified progress. User visual review remains the gate before 1C; do not mark that review passed without the user.
- [ ] Run `python3 scripts/check-docs.py`, `python3 scripts/source-descriptions.py check` and `git diff --check`. Commit `docs: record Phase 1B UI validation`; push the feature branch per the Git workflow. Do not merge, release, change version or begin 1C.

### Focused manual matrix

| Check | Passing observation |
| --- | --- |
| Welcome/preview/live boundary | Startup opens no cluster; explicit preview is always labeled; preview operations never contact Kubernetes; missing live data is not replaced by fixtures |
| Tab identity and confirmation | Same names across A/B do not mix state; namespace/all/cluster scope is clear; opening confirmation on A then selecting B never changes A target |
| Themes and settings | System responds to OS changes; Light/Dark persist after restart; rejected settings show warning; keyboard focus and selected rows remain distinguishable |
| Size/monitor changes | 1920×1080, 1440×900 and candidate 1100×700 remain usable; AI stays right; list/detail reachable; saved off-screen bounds recover; record actual monitor disconnect check or mark unrun |
| Keyboard/modal behavior | Arrow selection and commands affect exact row; input/editor/terminal typing unaffected; dialog traps/returns focus; Escape cancels; disabled commands explain why |
| Sessions and AI | Hiding preserves state; closing targets only owned sessions; AI retains A on B/settings navigation; citations/payload examples remain explicitly synthetic |
| Error states/accessibility | Loading, empty, forbidden, stale, disconnected, unsupported and missing-dependency differ; long names/hostile strings remain plain text; controls have accessible labels and no color-only meaning |

## Minimal test budget

Retain the existing frontend/session and Go suites. Add one small workspace-state test file (two or three readable behavior cases are fine) and two settings/geometry Go cases because persistence introduces a new input boundary. No snapshots, exhaustive UI component suites, coverage target or native automation framework. CSS/layout/accessibility behavior uses the focused manual matrix. Additional tests require an actual uncovered risk or regression, not a file-count rule.

## Separate-session implementation prompt

> Implement Phase 1B only using `docs/superpowers/plans/2026-11-06-phase-1b-ui-foundation.md` and its approved specification. Use Superpowers executing-plans, read AGENTS.md and the engineering/Git workflow, then verify the reviewed planning commit is in your checkout. Work on a feature branch; preserve unrelated work. Keep the new product UI preview explicitly synthetic and retain the validated 1A live interface. Follow the plain-English source description contract and minimal test budget. Implement one task at a time, run its focused checks and commit reviewable increments. Finish with the packaged UI walkthrough and an HTML validation report; commit and push. Keep operational notes in ignored .superpowers/sdd/, never the root README. Do not begin 1C, merge or release. Return the branch, commits, actual checks, limitations and screenshots for user UI review.

## Plan self-review

- Spec coverage: shell/tabs → Tasks 1–2; details/review/sessions/keyboard → Task 3; persistence/themes/geometry → Task 4; AI preview → Task 5; acceptance/handoff → Task 6.
- Live/preview interfaces are explicit; no new Kubernetes APIs or provider clients are required.
- Test additions cover target/state and settings risks; visual/manual checks cover layout and input focus without broad new suites.
- Candidate minimum dimensions require measured validation, not an invented compatibility claim.
- User approved the UI design; this planning session does not perform implementation.
