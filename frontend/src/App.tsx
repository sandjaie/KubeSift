/*
Purpose: Compose the real cluster list, Settings and independently owned live tabs.
Used by: The production React entry point.
Responsibilities: Preserve appearance/layout and keep global navigation separate from connection targets.
Important: Startup loads local summaries only; development validation and synthetic fixtures are excluded.
*/
import { useEffect, useRef, useState, type CSSProperties } from "react";
import { Service } from "../bindings/internal/desktop";
import { useConnections } from "./connections/useConnections";
import { ClusterList } from "./connections/ClusterList";
import { ConnectionControls } from "./connections/ConnectionControls";
import type { CatalogEntry } from "./connections/api";
import { SettingsPage } from "./settings/SettingsPage";
import { SourcesSettings } from "./settings/SourcesSettings";
import { TrustSettings } from "./settings/TrustSettings";
import { defaultPreferences, loadPreferences, savePreferences, type Preferences } from "./settings/preferences";
import { ClusterTabs } from "./workspace/ClusterTabs";
import { NamespaceControl } from "./workspace/NamespaceControl";
import { HelmWorkspace } from "./helm/HelmWorkspace";
import { useHelm } from "./helm/useHelm";
import { Workspace } from "./workspace/Workspace";
import { CommandPalette } from "./workspace/CommandPalette";
import { commandForKey } from "./workspace/commands";
import { Modal } from "./ui/ReviewDialog";
import { ResizeHandle } from "./ui/ResizeHandle";
import { ResourceIcon } from "./ui/ResourceIcon";
import { InvestigationPanel } from "./ai/InvestigationPanel";
import { useLogSessions } from "./sessions/useLogSessions";
import { PodLogs } from "./sessions/PodLogs";
import { InteractiveSessions } from "./sessions/InteractiveSessions";
import { forwardStart, terminalStart, useInteractiveSessions } from "./sessions/useInteractiveSessions";
import { sameConnection } from "./sessions/interactiveState";
import { TerminalStart } from "./sessions/TerminalStart";
import { ForwardStart } from "./sessions/ForwardStart";
import { useWorkloads } from "./workloads/useWorkloads";
import type { ResourceDetail, ResourceRef } from "./workloads/model";
import { requestEditor, settleCreateEditor, type EditorSession, type EditorDestination } from "./operations/editState";
import { CreateDialog } from "./operations/CreateDialog";
import { ApplyResponseDialog } from "./operations/ApplyDialog";
import { applyEditorHidden, settleApplyEditor } from "./operations/applyState";
import { EditDialog } from "./operations/EditDialog";
import { closeApply, closeEdit, type CreatedResourceOpening } from "./operations/api";
import { OperationResults, useOperationResults } from "./operations/OperationResults";
import "./styles.css";
export default function App() {
  const controller = useConnections();
  const logs = useLogSessions();
  const interactive = useInteractiveSessions();
  const [dockOpen, setDockOpen] = useState(false);
  const [dockSession, setDockSession] = useState("");
  const [startSession, setStartSession] = useState<{ kind: "terminal" | "forward"; data: ResourceDetail; tab: string; cluster: string }>();
  const { state } = controller;
  const [page, setPage] = useState<"clusters" | "workspace" | "settings">("clusters");
  const [aiOpen, setAiOpen] = useState(false);
  const [collapsedResourceGroups, setCollapsedResourceGroups] = useState<string[]>([]);
  const [hiddenNavigation, setHiddenNavigation] = useState<Record<string, boolean>>({});
  const aiButton = useRef<HTMLButtonElement>(null);
  const [palette, setPalette] = useState(false);
  const [trustTarget, setTrustTarget] = useState<CatalogEntry | null>(null);
  const [preferences, setPreferences] = useState(defaultPreferences);
  const prefsRef = useRef(preferences);
  prefsRef.current = preferences;
  const [settingsError, setSettingsError] = useState("");
  const [saving, setSaving] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const queue = useRef(Promise.resolve());
  function persist(value: Preferences) {
    setPreferences(value);
    setSaving(true);
    queue.current = queue.current
      .then(() => savePreferences(value))
      .then(() => setSettingsError(""))
      .catch(() =>
        setSettingsError("Could not save preferences. Check the local configuration file and permissions; your choice is only in memory."),
      )
      .finally(() => setSaving(false));
  }
  function resize(field: "navigationWidth" | "aiWidth" | "dockHeight" | "detailWidthPercent" | "detailHeightPercent", value: number) {
    const next = { ...prefsRef.current, [field]: value };
    prefsRef.current = next;
    setPreferences(next);
  }
  function commitSize() {
    if (loaded && !settingsError) persist(prefsRef.current);
  }
  useEffect(() => {
    void loadPreferences()
      .then(setPreferences)
      .catch(() =>
        setSettingsError("Preferences could not be loaded. In-memory defaults are active; the existing file has not been overwritten."),
      )
      .finally(() => setLoaded(true));
  }, []);
  useEffect(() => {
    const media = matchMedia("(prefers-color-scheme: dark)");
    const apply = () => {
      document.documentElement.dataset.theme =
        preferences.appearance === "system" ? (media.matches ? "dark" : "light") : preferences.appearance;
    };
    apply();
    media.addEventListener("change", apply);
    return () => media.removeEventListener("change", apply);
  }, [preferences.appearance]);
  const [startupError, setStartupError] = useState("");
  useEffect(() => {
    void Service.Ping()
      .then(() => Service.FrontendReady())
      .catch(() => setStartupError("Desktop bridge unavailable"));
  }, []);
  useEffect(() => {
    const listener = (event: KeyboardEvent) => {
      if (commandForKey(event) === "palette") {
        event.preventDefault();
        setPalette(true);
      }
    };
    window.addEventListener("keydown", listener);
    return () => window.removeEventListener("keydown", listener);
  }, []);
  const active = state.active ? state.tabs[state.active] : null;
  const [helmDestinations, setHelmDestinations] = useState<Record<string, { mode: "helm" | "resource"; source?: string }>>({});
  const helmDestination = state.active ? helmDestinations[state.active] : undefined;
  const showingHelm = helmDestination?.mode === "helm";
  const helmNamespace = helmDestination?.source ?? active?.namespace ?? "default";
  const helmConnection = active?.view?.state === "connected" || active?.view?.state === "restricted" ? active.view.ref : undefined;
  const helm = useHelm(
    state.active ?? "",
    helmConnection ?? undefined,
    helmNamespace,
    showingHelm && page === "workspace" && active?.namespace === helmNamespace,
  );
  useEffect(() => {
    if (!showingHelm || !helmDestination?.source || active?.namespace !== helmDestination.source || !state.active) return;
    const tab = state.active;
    setHelmDestinations((old) => ({ ...old, [tab]: { mode: "helm" } }));
  }, [showingHelm, helmDestination?.source, active?.namespace, state.active]);
  const workloads = useWorkloads(
    state.active ?? "",
    active?.view?.ref ?? null,
    active?.namespace ?? "default",
    page === "workspace" && !showingHelm,
    (tab, namespace) => void controller.selectNamespace(tab, namespace),
  );
  const [editor, setEditor] = useState<EditorSession<EditorDestination>>();
  const origin = editor && state.tabs[editor.tab];
  const inOriginWorkspace = !!editor && editor.tab === state.active && page === "workspace";
  const editorHidden =
    editor?.mode === "apply" ? applyEditorHidden(editor.suspended, !!origin, inOriginWorkspace) : !!editor?.suspended || !inOriginWorkspace;
  const originConnection =
    origin?.view?.state === "connected" || origin?.view?.state === "restricted" ? (origin.view.ref ?? undefined) : undefined;
  const operationResults = useOperationResults((result) => {
    if (result.state !== "accepted") return;
    workloads.refreshOperationTarget(result.resource, result.action === "delete");
  });
  function resumeEditor() {
    if (!editor || editor.submitting) return;
    if (state.tabs[editor.tab]) {
      controller.commit({ type: "focus", key: editor.tab });
      setPage("workspace");
    }
    setEditor({ ...editor, suspended: false });
  }
  const [resultsOpen, setResultsOpen] = useState(false);
  const [openingOperationTarget, setOpeningOperationTarget] = useState<{
    ref: ResourceRef;
    tab: string;
    created?: CreatedResourceOpening;
  } | null>(null);
  const operationConnectionKey = (ref: ResourceRef["connection"]) => JSON.stringify([ref.id, ref.generation]);
  const findOperationTab = (resource: ResourceRef) =>
    Object.entries(state.tabs).find(
      ([, tab]) => tab.view?.ref && operationConnectionKey(tab.view.ref) === operationConnectionKey(resource.connection),
    )?.[0];
  useEffect(() => {
    if (!openingOperationTarget || page !== "workspace" || state.active !== openingOperationTarget.tab || !workloads.ready) return;
    setOpeningOperationTarget(null);
    workloads.openOperationTarget(openingOperationTarget.ref, openingOperationTarget.created);
  }, [openingOperationTarget, page, state.active, workloads.ready]);
  function choose(entry: CatalogEntry) {
    setPage("workspace");
    void controller.connect(entry);
  }
  const [closeTarget, setCloseTarget] = useState<{ key: string; entries: string[] } | null>(null);
  function close(key: string) {
    if (editor?.tab === key && !editor.submitting) {
      setEditor({ ...editor, suspended: false, closing: true });
      return;
    }
    closeTab(key);
  }
  function completeTabClose(key: string) {
    void Promise.all([logs.store.closeTab(key), interactive.store.closeTab(key)])
      .then(() => {
        workloads.close(key);
        helm.closeTab(key);
        setHelmDestinations((old) => {
          const next = { ...old };
          delete next[key];
          return next;
        });
        return controller.cancel(key);
      })
      .catch(() => setStartupError("Could not close the tab’s sessions. Retry cleanup."));
  }
  function closeTab(key: string) {
    const sessions = interactive.entries.filter((entry) => entry.tab === key);
    const labels = [
      ...logs.sessions.filter((entry) => entry.tab === key).map((entry) => `Logs · ${entry.request.resource.name} · ${entry.status}`),
      ...sessions.map((entry) => `${entry.kind === "terminal" ? "Terminal" : "Forward"} · ${entry.target.resource.name} · ${entry.state}`),
    ];
    const pending = interactive.store.countTab(key) - sessions.length;
    if (pending > 0) labels.push(`${pending} session start${pending === 1 ? "" : "s"} in progress`);
    if (labels.length) setCloseTarget({ key, entries: labels });
    else completeTabClose(key);
  }
  useEffect(() => {
    interactive.store.syncConnections((tab, ref) => {
      const view = state.tabs[tab]?.view;
      return (view?.state === "connected" || view?.state === "restricted") && sameConnection(ref, view.ref);
    });
  }, [interactive.store, state.tabs]);
  const sessionOrigin = startSession && state.tabs[startSession.tab]?.view;
  const canStartSession =
    startSession &&
    page === "workspace" &&
    state.active === startSession.tab &&
    (sessionOrigin?.state === "connected" || sessionOrigin?.state === "restricted") &&
    sameConnection(startSession.data.resource.connection, sessionOrigin.ref);
  useEffect(() => {
    if (startSession && !canStartSession) setStartSession(undefined);
  }, [startSession, canStartSession]);
  useEffect(() => {
    for (const session of logs.sessions) {
      const ref = state.tabs[session.tab]?.view?.ref;
      if (
        !ref ||
        ref.id !== session.request.resource.connection.id ||
        ref.generation !== session.request.resource.connection.generation ||
        state.tabs[session.tab]?.view?.state !== "connected"
      )
        logs.store.disconnected(session.tab);
    }
  }, [state.tabs, logs.store, logs.sessions]);
  function settings() {
    setTrustTarget(null);
    setPage("settings");
  }
  return (
    <div
      className="product"
      style={
        {
          "--nav-width": `${preferences.navigationWidth}px`,
          "--ai-width": `${preferences.aiWidth}px`,
          "--dock-height": `${preferences.dockHeight}px`,
        } as CSSProperties
      }
    >
      <header className="topbar compact-header">
        <button className="brand" aria-label="KubeSift clusters" onClick={() => setPage("clusters")}>
          <ResourceIcon kind="Pod" size={23} /> KubeSift
        </button>
        <ClusterTabs
          tabs={state.tabs}
          active={page === "workspace" ? state.active : null}
          onSelect={(key) => {
            controller.commit({ type: "focus", key });
            setPage("workspace");
          }}
          onClose={close}
        />
        <button onClick={() => setPage("clusters")}>Choose cluster</button>
        {page === "workspace" && helmDestination?.mode === "resource" && (
          <button
            onClick={() => {
              const tab = state.active;
              if (!tab) return;
              setHelmDestinations((old) => ({ ...old, [tab]: { mode: "helm", source: helmDestination.source } }));
              if (helmDestination.source && active?.namespace !== helmDestination.source) {
                void controller.selectNamespace(tab, helmDestination.source);
              }
            }}
          >
            Back to release
          </button>
        )}
        {editor && editorHidden && (
          <button disabled={editor.submitting} onClick={resumeEditor}>
            {editor.submitting
              ? "Submitting operation…"
              : editor.mode === "create"
                ? "Resume create draft"
                : editor.mode === "apply"
                  ? editor.observation
                    ? "View apply response"
                    : "Resume apply draft"
                  : "Resume YAML edit"}
          </button>
        )}
        <button
          onClick={() => {
            operationResults.refresh();
            setResultsOpen(true);
          }}
        >
          Operation results
          {operationResults.pending ? " · Running" : operationResults.results.length ? ` (${operationResults.results.length})` : ""}
        </button>
        <button
          aria-pressed={page === "settings"}
          onClick={() => (page === "settings" ? setPage(active ? "workspace" : "clusters") : settings())}
        >
          {page === "settings" ? "Back to workspace" : "Settings"}
        </button>
        <button
          ref={aiButton}
          className="primary"
          aria-expanded={aiOpen}
          aria-controls="ai-investigation-panel"
          onClick={() => setAiOpen(!aiOpen)}
        >
          {aiOpen ? "Hide AI" : "Ask AI"}
        </button>
      </header>
      {editor?.mode === "create" && (
        <CreateDialog
          key={editor.id}
          destination={editor}
          context={origin?.entry.context ?? "Original connection"}
          connection={originConnection}
          suspended={!editor.replacement && !editor.closing && editorHidden}
          closing={editor.closing || !!editor.replacement}
          replacing={!!editor.replacement}
          onSuspend={() => setEditor({ ...editor, suspended: true })}
          onCancelClose={() => setEditor({ ...editor, closing: false, replacement: undefined })}
          onClose={() => {
            setEditor(editor.replacement ? requestEditor(undefined, editor.replacement) : undefined);
            if (editor.closing) closeTab(editor.tab);
          }}
          onConfirm={(token) => {
            setEditor({ ...editor, suspended: true, submitting: true });
            setResultsOpen(true);
            void operationResults.submit("create", token, "").then((result) => {
              setEditor((current) => settleCreateEditor(current, editor.id, result?.state === "accepted"));
            });
          }}
        />
      )}
      {editor?.mode === "apply" && editor.observation && !editorHidden && (
        <ApplyResponseDialog
          observation={editor.observation}
          onClose={() => {
            setEditor(undefined);
            if (editor.closing) closeTab(editor.tab);
          }}
        />
      )}
      {editor && editor.mode !== "create" && !(editor.mode === "apply" && editor.observation) && (
        <EditDialog
          key={editor.mode === "apply" ? editor.id : JSON.stringify([editor.tab, editor.target])}
          apply={editor.mode === "apply"}
          target={editor.target}
          context={origin?.entry.context ?? "Original connection"}
          connection={originConnection}
          suspended={!editor.replacement && !editor.closing && editorHidden}
          reveal={editor.reveal}
          closing={editor.closing || !!editor.replacement}
          replacing={!!editor.replacement}
          onSuspend={() => setEditor({ ...editor, suspended: true })}
          onCancelClose={() => setEditor({ ...editor, closing: false, replacement: undefined })}
          onClose={() => {
            setEditor(editor.replacement ? requestEditor(undefined, editor.replacement) : undefined);
            if (editor.closing) closeTab(editor.tab);
          }}
          onConfirm={(token, draftID, reviewedServerYAML) => {
            if (editor.mode === "apply") {
              setEditor({ ...editor, suspended: true, submitting: true });
              setResultsOpen(true);
              void operationResults.submit("apply", token, "").then((result) => {
                const accepted = result?.state === "accepted";
                if (accepted) closeApply(draftID);
                const observed =
                  result && "observedYAML" in result && typeof result.observedYAML === "string" ? result.observedYAML : undefined;
                const observation =
                  accepted && result && "responseChanged" in result && result.responseChanged && reviewedServerYAML !== undefined
                    ? { before: observed ? reviewedServerYAML : "", observed }
                    : undefined;
                setEditor((current) => settleApplyEditor(current, editor.id, accepted, observation));
              });
              return;
            }
            void operationResults.submit("edit", token, "").finally(() => closeEdit(draftID));
            setEditor(undefined);
            setResultsOpen(true);
          }}
        />
      )}
      {resultsOpen && (
        <OperationResults
          results={operationResults.results}
          pending={operationResults.pending}
          error={operationResults.error}
          onClose={() => setResultsOpen(false)}
          onRefresh={operationResults.refresh}
          canOpen={(ref) => !!findOperationTab(ref)}
          onTarget={(ref, created) => {
            const tab = findOperationTab(ref);
            if (!tab) return;
            controller.commit({ type: "focus", key: tab });
            setPage("workspace");
            setOpeningOperationTarget({ ref, tab, created });
          }}
        />
      )}
      <div className="identity-bar compact-identity">
        {page === "workspace" && active ? (
          <>
            <button
              aria-label="Toggle resource navigation"
              aria-expanded={!hiddenNavigation[state.active!]}
              title="Show or hide resources"
              onClick={() => setHiddenNavigation((old) => ({ ...old, [state.active!]: !old[state.active!] }))}
            >
              ☰
            </button>
            <strong title={active.entry.sourceDisplay}>{active.entry.context}</strong>
            {workloads.clusterScoped && <span className="muted">Cluster-scoped</span>}
            <NamespaceControl
              key={state.active}
              namespace={active.namespace}
              names={active.view?.namespaces ?? []}
              complete={active.view?.namespacesComplete ?? false}
              disabled={
                workloads.clusterScoped ||
                !active.view?.ref ||
                active.view.state === "connecting" ||
                (!!active.view.problem && active.view.problem.code !== "forbidden")
              }
              onSelect={(namespace) => void controller.selectNamespace(state.active!, namespace)}
            />
            <span className="spacer" />
            <ConnectionControls
              key={`connection-${state.active}`}
              tab={active}
              onRetry={() => void controller.retry(state.active!)}
              onCancel={() => close(state.active!)}
              onReview={() => {
                setTrustTarget(active.entry);
                setPage("settings");
              }}
            />
          </>
        ) : (
          <>
            <strong>{page === "settings" ? "Settings" : "Clusters"}</strong>
            <span className="muted">{Object.keys(state.tabs).length} open contexts</span>
          </>
        )}
        {startupError && <span role="alert">{startupError}</span>}
      </div>
      {startSession &&
        canStartSession &&
        (startSession.kind === "terminal" ? (
          <TerminalStart
            data={startSession.data}
            cluster={startSession.cluster}
            onClose={() => setStartSession(undefined)}
            onStart={async (request, signal) => {
              const entry = await interactive.store.open(
                startSession.tab,
                startSession.cluster,
                request.target.resource,
                terminalStart(request),
                signal,
              );
              setDockSession(entry.session.id);
              setDockOpen(true);
            }}
          />
        ) : (
          <ForwardStart
            data={startSession.data}
            cluster={startSession.cluster}
            onClose={() => setStartSession(undefined)}
            onStart={async (request, signal) => {
              const entry = await interactive.store.open(
                startSession.tab,
                startSession.cluster,
                request.target.resource,
                forwardStart(request),
                signal,
              );
              setDockSession(entry.session.id);
              setDockOpen(true);
            }}
          />
        ))}
      <div className="main-columns">
        <div className="page-body">
          {page === "settings" ? (
            <SettingsPage
              preferences={preferences}
              error={settingsError}
              saving={saving || !loaded}
              onChange={persist}
              onReset={() => persist(defaultPreferences())}
            >
              <SourcesSettings controller={controller} />
              <TrustSettings
                key={trustTarget ? JSON.stringify([trustTarget.sourceId, trustTarget.context]) : "general"}
                controller={controller}
                target={trustTarget}
                onConnect={(entry) => {
                  setPage("workspace");
                  const key = JSON.stringify([entry.sourceId, entry.context]);
                  if (state.tabs[key]) void controller.retry(key);
                  else void controller.connect(entry);
                }}
              />
            </SettingsPage>
          ) : page === "workspace" && active ? (
            <>
              {(active.error || active.view?.problem || active.pollFailed) && (
                <div role="alert" className="connection-problem">
                  {active.error ||
                    active.view?.problem?.message ||
                    (active.pollFailed ? "Status unavailable; displayed data may be stale." : "")}
                </div>
              )}
              {workloads.navigationError && !workloads.selection && (
                <div role="alert" className="connection-problem">
                  {workloads.navigationError}
                </div>
              )}
              <Workspace
                key={state.active}
                tab={active}
                navigationOpen={!hiddenNavigation[state.active!]}
                collapsedResourceGroups={collapsedResourceGroups}
                onToggleResourceGroup={(group) =>
                  setCollapsedResourceGroups((current) =>
                    current.includes(group) ? current.filter((item) => item !== group) : [...current, group],
                  )
                }
                controller={workloads}
                helmPane={
                  (showingHelm && (
                    <HelmWorkspace
                      helm={helm}
                      namespace={helmNamespace}
                      widthPercent={preferences.detailWidthPercent}
                      heightPercent={preferences.detailHeightPercent}
                      onResize={(axis, value) => resize(axis === "x" ? "detailWidthPercent" : "detailHeightPercent", value)}
                      onCommit={commitSize}
                      onOpen={({ address, resource }) => {
                        const tab = state.active;
                        if (!tab) return;
                        setHelmDestinations((old) => ({ ...old, [tab]: { mode: "resource", source: helmNamespace } }));
                        setOpeningOperationTarget({ tab, ref: resource, created: { action: "helm", target: address } });
                      }}
                    />
                  )) ||
                  undefined
                }
                onHelm={() => {
                  const tab = state.active;
                  if (tab) setHelmDestinations((old) => ({ ...old, [tab]: { mode: "helm" } }));
                }}
                onResourceDestination={() => {
                  const tab = state.active;
                  if (tab)
                    setHelmDestinations((old) => {
                      const next = { ...old };
                      delete next[tab];
                      return next;
                    });
                }}
                operationResults={operationResults.results}
                onInteractive={(kind, data) => {
                  if (state.active) setStartSession({ kind, data, tab: state.active, cluster: active.entry.context });
                }}
                onCreate={
                  editor?.submitting
                    ? undefined
                    : () => {
                        const tab = state.active;
                        if (tab && active.view?.ref) {
                          const destination = {
                            mode: "create" as const,
                            id: crypto.randomUUID(),
                            tab,
                            connection: active.view.ref,
                            namespace: workloads.clusterScoped ? "" : active.namespace,
                          };
                          setEditor((current) => requestEditor(current, destination));
                        }
                      }
                }
                onEdit={
                  editor?.submitting
                    ? undefined
                    : (target, reveal = false) => {
                        const tab = state.active;
                        if (tab) setEditor((current) => requestEditor(current, { tab, target, reveal }));
                      }
                }
                onApply={
                  editor?.submitting
                    ? undefined
                    : (target) => {
                        const tab = state.active;
                        if (!tab) return;
                        const id = crypto.randomUUID();
                        setEditor((current) => requestEditor(current, { mode: "apply", id, tab, target, reveal: false }));
                      }
                }
                onOperation={(action, token, acknowledgement) => {
                  const releaseDeletion = action === "delete" ? workloads.holdDeletionSelection() : undefined;
                  operationResults.submit(action, token, acknowledgement, releaseDeletion);
                  setResultsOpen(true);
                }}
                logs={
                  workloads.selection?.resource.kind === "Pod" && workloads.activeSection === "Logs" ? (
                    <PodLogs
                      key={JSON.stringify(workloads.selection.resource)}
                      resource={workloads.selection.resource}
                      tab={state.active!}
                      label={active.entry.context}
                      controller={logs}
                    />
                  ) : undefined
                }
                detailWidthPercent={preferences.detailWidthPercent}
                detailHeightPercent={preferences.detailHeightPercent}
                onDetailResize={(axis, value) => resize(axis === "x" ? "detailWidthPercent" : "detailHeightPercent", value)}
                navigationWidth={preferences.navigationWidth}
                onResize={(n) => resize("navigationWidth", n)}
                onCommitSize={commitSize}
              />
            </>
          ) : (
            <ClusterList
              sources={state.sources}
              connections={state.tabs}
              loading={state.loading}
              error={state.error}
              onSelect={choose}
              onSettings={settings}
            />
          )}
        </div>
        <div id="ai-investigation-panel" hidden={!aiOpen} className="ai-host">
          <ResizeHandle
            label="AI panel width"
            reverse
            value={preferences.aiWidth}
            min={300}
            max={560}
            onChange={(n) => resize("aiWidth", n)}
            onCommit={commitSize}
          />
          <InvestigationPanel
            onClose={() => {
              setAiOpen(false);
              aiButton.current?.focus();
            }}
          />
        </div>
      </div>
      <InteractiveSessions
        controller={interactive}
        logs={logs}
        expanded={dockOpen}
        selected={dockSession}
        height={preferences.dockHeight}
        onResize={(height) => resize("dockHeight", height)}
        onCommit={commitSize}
        onToggle={() => setDockOpen((value) => !value)}
        onSelect={setDockSession}
      />
      {closeTarget && (
        <Modal title="Close cluster tab?" onCancel={() => setCloseTarget(null)}>
          <p>Close {state.tabs[closeTarget.key]?.entry.context} and these sessions?</p>
          <ul>
            {closeTarget.entries.map((entry, index) => (
              <li key={index}>{entry}</li>
            ))}
          </ul>
          <button
            className="primary"
            onClick={() => {
              const key = closeTarget.key;
              setCloseTarget(null);
              completeTabClose(key);
            }}
          >
            Close tab and sessions
          </button>
        </Modal>
      )}
      {logs.sessions
        .filter((session) => session.status === "cleanup-error")
        .map((session) => (
          <div role="alert" key={session.id}>
            {session.error}
            <button
              onClick={() => {
                void logs.store
                  .close(session.id)
                  .catch(() => setStartupError("Log cleanup failed; reconnect the cluster to end its readers."));
              }}
            >
              Retry cleanup
            </button>
          </div>
        ))}
      {palette && <CommandPalette available={false} onCommand={() => setPage("clusters")} onClose={() => setPalette(false)} />}
    </div>
  );
}
