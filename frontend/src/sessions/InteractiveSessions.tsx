/*
Purpose: Present retained sessions in the workspace's existing bottom dock.
Used by: App across clusters, details and Settings.
Responsibilities: Keep terminal renderers mounted and expose explicit stop, close and copy actions.
Important: Only the selected visible terminal accepts keyboard input.
*/
import { lazy, Suspense, useLayoutEffect } from "react";
import { ResizeHandle } from "../ui/ResizeHandle";
import { CopyTextButton } from "../ui/CopyTextButton";
import type { InteractiveSessionsController } from "./useInteractiveSessions";
import type { LogSessionsController } from "./useLogSessions";
import { interactiveActive } from "./interactiveState";

const TerminalView = lazy(() => import("../Terminal"));

export function InteractiveSessions({
  controller,
  logs,
  expanded,
  selected,
  height,
  onResize,
  onCommit,
  onToggle,
  onSelect,
}: {
  controller: InteractiveSessionsController;
  logs: LogSessionsController;
  expanded: boolean;
  selected: string;
  height: number;
  onResize: (height: number) => void;
  onCommit: () => void;
  onToggle: () => void;
  onSelect: (id: string) => void;
}) {
  const { entries, store } = controller;
  const logEntry = logs.sessions.find((entry) => entry.id === selected);
  const selectedID =
    entries.some((entry) => entry.session.id === selected) || logEntry
      ? selected
      : (entries[entries.length - 1]?.session.id ?? logs.sessions[logs.sessions.length - 1]?.id);
  useLayoutEffect(() => {
    for (const entry of entries) if (entry.kind === "forward") store.activate(entry.session);
  }, [entries, store]);
  const count = entries.length + logs.sessions.length;
  if (!count) return null;

  return (
    <section className="sessions-dock" aria-label="Sessions">
      {expanded && (
        <ResizeHandle label="Sessions height" value={height} min={120} max={480} axis="y" reverse onChange={onResize} onCommit={onCommit} />
      )}
      <header>
        <button aria-expanded={expanded} aria-controls="product-sessions" onClick={onToggle}>
          Sessions ({count})
        </button>
        <span className="muted">{expanded ? "Session targets stay fixed" : "Collapsed"}</span>
      </header>
      <div className="product-sessions interactive-dock" id="product-sessions" hidden={!expanded}>
        <div className="session-tabs" aria-label="Retained sessions">
          {entries.map((entry) => (
            <button key={entry.session.id} aria-pressed={selectedID === entry.session.id} onClick={() => onSelect(entry.session.id)}>
              {entry.kind === "terminal" ? "Terminal" : "Forward"} · {entry.cluster} · {entry.target.resource.name}
            </button>
          ))}
          {logs.sessions.map((entry) => (
            <button key={entry.id} aria-pressed={selectedID === entry.id} onClick={() => onSelect(entry.id)}>
              Logs · {entry.label} · {entry.request.resource.name}
            </button>
          ))}
        </div>
        {entries.map((entry) => {
          const id = entry.session.id;
          const target = entry.target.resource;
          const label = [entry.cluster, target.namespace, target.name, entry.target.container].filter(Boolean).join(" / ");
          const visible = expanded && selectedID === id;
          const running = interactiveActive(entry.state);
          const address = entry.state === "listening" && entry.localPort ? `127.0.0.1:${entry.localPort}` : "";
          return (
            <article key={id} hidden={selectedID !== id} className="session-entry" aria-label={label}>
              <div className="session-summary">
                <strong>{label}</strong>
                <span role="status">{entry.state}</span>
                {entry.kind === "forward" && (
                  <span>
                    {entry.service ? `Service ${entry.service.resource.name}:${entry.service.port} → ` : ""}Pod {target.name} · TCP{" "}
                    {entry.remotePort}
                  </span>
                )}
                {address && (
                  <>
                    <code>{address}</code>
                    <CopyTextButton text={address} label="Copy address" copiedLabel="Address copied" />
                  </>
                )}
                {(running || entry.state === "cleanup-error") && <button onClick={() => void store.stop(id)}>Stop</button>}
                <button
                  disabled={entry.state === "closing" || entry.state === "stopping"}
                  onClick={() => void store.close(id).catch(() => {})}
                >
                  Close session
                </button>
              </div>
              {entry.problem && <p role="alert">{entry.problem.message}</p>}
              {entry.kind === "terminal" && (
                <Suspense fallback={<p role="status">Loading terminal…</p>}>
                  <TerminalView
                    managed
                    session={entry.session}
                    label={label}
                    visible={visible}
                    active={running}
                    onAttach={store.activate}
                  />
                </Suspense>
              )}
            </article>
          );
        })}
        {logEntry && (
          <article className="session-entry">
            <div className="session-summary">
              <strong>
                {logEntry.label} / {logEntry.request.resource.namespace} / {logEntry.request.resource.name} / {logEntry.request.container}
              </strong>
              <span>{logEntry.status}</span>
              {logEntry.active && <button onClick={() => void logs.store.stop(logEntry.id).catch(() => {})}>Stop</button>}
              <button onClick={() => void logs.store.close(logEntry.id).catch(() => {})}>Close session</button>
            </div>
            <p className="muted">Open the original Pod’s Logs section to view log evidence.</p>
            {logEntry.error && <p role="alert">{logEntry.error}</p>}
          </article>
        )}
      </div>
    </section>
  );
}
