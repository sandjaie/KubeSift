/*
Purpose: Compose the real cluster list, Settings and independently owned live tabs.
Used by: The production React entry point.
Responsibilities: Preserve appearance/layout and keep global navigation separate from connection targets.
Important: Startup loads local summaries only; development validation and synthetic fixtures are excluded.
*/
import { useEffect, useRef, useState, type CSSProperties } from "react";
import { Service } from "../bindings/github.com/sandjaie/KubeSift/internal/desktop";
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
import { Workspace } from "./workspace/Workspace";
import { CommandPalette } from "./workspace/CommandPalette";
import { commandForKey } from "./workspace/commands";
import { Modal } from "./ui/ReviewDialog";
import { ResizeHandle } from "./ui/ResizeHandle";
import { ResourceIcon } from "./ui/ResourceIcon";
import { InvestigationPanel } from "./ai/InvestigationPanel";
import { useLogSessions } from "./sessions/useLogSessions";
import { PodLogs } from "./sessions/PodLogs";
import { SessionsDock } from "./sessions/SessionsDock";
import { useWorkloads } from "./workloads/useWorkloads";
import "./styles.css";
export default function App() {
  const controller = useConnections();
  const logs = useLogSessions();
  const { state } = controller;
  const [page, setPage] = useState<"clusters" | "workspace" | "settings">("clusters");
  const [aiOpen, setAiOpen] = useState(false);
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
  const workloads = useWorkloads(state.active ?? "", active?.view?.ref ?? null, active?.namespace ?? "default", page === "workspace");
  function choose(entry: CatalogEntry) {
    setPage("workspace");
    void controller.connect(entry);
  }
  const [closeTarget, setCloseTarget] = useState<{ key: string; count: number } | null>(null);
  function close(key: string) {
    const count = logs.sessions.filter((s) => s.tab === key).length;
    if (count) {
      setCloseTarget({ key, count });
      return;
    }
    void logs.store.closeTab(key).then(() => {
      workloads.close(key);
      void controller.cancel(key);
    });
  }
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
            <NamespaceControl
              key={state.active}
              namespace={active.namespace}
              names={active.view?.namespaces ?? []}
              complete={active.view?.namespacesComplete ?? false}
              disabled={
                !active.view?.ref ||
                active.view.state === "connecting" ||
                (!!active.view.problem && active.view.problem.code !== "forbidden")
              }
              onSelect={(namespace) => void controller.readPods(state.active!, namespace)}
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
              <Workspace
                key={state.active}
                tab={active}
                navigationOpen={!hiddenNavigation[state.active!]}
                controller={workloads}
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
      {closeTarget && (
        <Modal title="Close cluster tab?" onCancel={() => setCloseTarget(null)}>
          <p>
            Close {state.tabs[closeTarget.key]?.entry.context} and its {closeTarget.count} retained log sessions?
          </p>
          <button
            className="primary"
            onClick={() => {
              const key = closeTarget.key;
              setCloseTarget(null);
              void logs.store
                .closeTab(key)
                .then(() => {
                  workloads.close(key);
                  void controller.cancel(key);
                })
                .catch(() => setStartupError("Could not close log sessions; retry closure."));
            }}
          >
            Close tab and sessions
          </button>
        </Modal>
      )}
      <SessionsDock
        controller={logs}
        connections={Object.fromEntries(Object.entries(state.tabs).map(([id, tab]) => [id, tab.view?.ref ?? null]))}
      />
      {palette && <CommandPalette available={false} onCommand={() => setPage("clusters")} onClose={() => setPalette(false)} />}
    </div>
  );
}
