/*
Purpose: Render a bounded window of shared retained log evidence.
Used by: Pod Logs and the global Sessions dock.
Responsibilities: Separate Stop, Clear, scroll pause and explicit Resume with gap disclosure.
Important: Rendering never subscribes or opens a native log reader.
*/
import { useEffect, useRef, useState } from "react";
import { displayWindow, scrollLogViewport, type LogSession, type LogStore } from "./logStore";
import { Modal } from "../ui/ReviewDialog";
import type { ConnectionRef } from "../bridge";
export function LogViewer({
  session: s,
  store,
  connection,
  embedded = false,
}: {
  session: LogSession;
  store: LogStore;
  connection?: ConnectionRef;
  embedded?: boolean;
}) {
  const [follow, setFollow] = useState(true),
    [offset, setOffset] = useState(0),
    [query, setQuery] = useState(""),
    [notice, setNotice] = useState("");
  const viewport = useRef<HTMLPreElement>(null);
  const [resumeTarget, setResumeTarget] = useState<{ connection?: ConnectionRef; changed: boolean } | null>(null);
  useEffect(() => scrollLogViewport(viewport.current, follow), [s.text, follow]);
  const bytes = new TextEncoder().encode(s.text).length;
  const start = follow ? Math.max(0, bytes - 65536) : Math.min(offset, Math.max(0, bytes - 1));
  const attempt = (fn: () => Promise<unknown>) => {
    void fn().catch(() => setNotice("Session action failed; retry or close the session."));
  };
  return (
    <div className="log-viewer">
      {resumeTarget && (
        <Modal title="Resume log session?" onCancel={() => setResumeTarget(null)}>
          <p>
            {resumeTarget.changed ? "Confirm the newly connected target. " : ""}
            {s.label} / {s.request.resource.namespace} / {s.request.resource.name} / {s.request.container}
          </p>
          <p>The original Pod is checked again before resuming. Logs may overlap or contain a gap.</p>
          <button
            className="primary"
            onClick={() => {
              const target = resumeTarget.connection;
              setResumeTarget(null);
              attempt(() => store.resume(s.id, target));
            }}
          >
            Resume logs
          </button>
        </Modal>
      )}
      {!embedded && (
        <p className="log-target">
          {s.label} · {s.request.resource.namespace} · {s.request.resource.name}
        </p>
      )}
      <div className="log-toolbar">
        <strong className="log-session-name">
          {s.request.container} · {s.request.previous ? "Previous" : "Current"}
        </strong>
        <span className="log-status">
          {s.status === "complete" ? "Loaded" : s.status === "running" ? (s.request.follow ? "Live" : "Loading") : s.status}
        </span>
        {s.active && <button onClick={() => attempt(() => store.stop(s.id))}>Stop</button>}
        <button disabled={!s.text} onClick={() => store.clear(s.id)}>
          Clear view
        </button>
        {!s.active && (
          <button
            onClick={() => {
              const changed =
                connection &&
                (connection.id !== s.request.resource.connection.id || connection.generation !== s.request.resource.connection.generation);
              setResumeTarget({ connection, changed: !!changed });
            }}
          >
            Resume
          </button>
        )}
        <button onClick={() => attempt(() => store.close(s.id))}>Close</button>
        <label className="log-checkbox">
          <input
            type="checkbox"
            checked={follow}
            onChange={(e) => {
              setOffset(start);
              setFollow(e.target.checked);
            }}
          />{" "}
          Auto-scroll
        </label>
      </div>
      <details className="log-search">
        <summary>Search logs</summary>
        <div className="log-toolbar">
          <input placeholder="Search logs…" aria-label="Search retained logs" value={query} onChange={(e) => setQuery(e.target.value)} />
          <button
            disabled={!query}
            onClick={() => {
              const index = s.text.toLowerCase().indexOf(query.toLowerCase());
              if (index >= 0) {
                setOffset(new TextEncoder().encode(s.text.slice(0, index)).length);
                setFollow(false);
                setNotice("First match in retained text");
              } else setNotice("No match in retained text");
            }}
          >
            Find
          </button>
        </div>
      </details>
      {notice && <p role="status">{notice}</p>}
      {s.dropped > 0 && (
        <p className="warning" role="status">
          Some earlier log text is unavailable ({s.dropped.toLocaleString()} bytes omitted).
        </p>
      )}
      {(s.loss || s.error) && (
        <p className="warning" role="status">
          {s.error || s.loss}
        </p>
      )}
      <pre ref={viewport} className="log-text">
        {displayWindow(s.text, start) || (s.active ? "Waiting for logs…" : "No log output.")}
      </pre>
      {bytes > 65536 && (
        <nav className="log-toolbar" aria-label="Log pages">
          <button
            disabled={start === 0}
            onClick={() => {
              setFollow(false);
              setOffset(Math.max(0, start - 65536));
            }}
          >
            Earlier
          </button>
          <button
            disabled={start + 65536 >= bytes}
            onClick={() => {
              setFollow(false);
              setOffset(start + 65536);
            }}
          >
            Later
          </button>
        </nav>
      )}
    </div>
  );
}
