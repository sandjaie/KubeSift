/*
Purpose: Keep actual log sessions accessible across product navigation.
Used by: App below the workspace and Settings content.
Responsibilities: Collapse rendering without changing session ownership.
Important: Detail and dock share the same native stream and retained history.
*/
import { useState } from "react";
import type { LogSessionsController } from "./useLogSessions";
import type { ConnectionRef } from "../bridge";
import { LogViewer } from "./LogViewer";
export function SessionsDock({
  controller,
  connections,
}: {
  controller: LogSessionsController;
  connections: Record<string, ConnectionRef | null>;
}) {
  const [open, setOpen] = useState(false);
  return (
    <section className="sessions-dock product-sessions">
      <header>
        <button aria-expanded={open} onClick={() => setOpen(!open)}>
          {open ? "▾" : "▸"} Sessions · {controller.sessions.length}
        </button>
        <span className="muted">Targets stay fixed across navigation</span>
      </header>
      {open && (
        <div className="session-cards">
          {!controller.sessions.length && <p>No log sessions. Select a Pod, then Logs.</p>}
          {controller.sessions.map((s) => (
            <article key={s.id}>
              <LogViewer session={s} store={controller.store} connection={connections[s.tab] ?? undefined} />
            </article>
          ))}
        </div>
      )}
    </section>
  );
}
