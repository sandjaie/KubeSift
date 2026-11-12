/*
Purpose: Render a bounded window of shared retained log evidence.
Used by: Pod Logs and the global Sessions dock.
Responsibilities: Separate Stop, Clear, scroll pause and explicit Resume with gap disclosure.
Important: Rendering never subscribes or opens a native log reader.
*/
import { useState } from "react";
import { displayWindow, type LogSession, type LogStore } from "./logStore";
import type { ConnectionRef } from "../bridge";
export function LogViewer({ session: s, store, connection }: { session: LogSession; store: LogStore; connection?: ConnectionRef }) {
  const [follow, setFollow] = useState(true),
    [offset, setOffset] = useState(0),
    [query, setQuery] = useState(""),
    [notice, setNotice] = useState("");
  const bytes = new TextEncoder().encode(s.text).length;
  const start = follow ? Math.max(0, bytes - 65536) : Math.min(offset, Math.max(0, bytes - 1));
  const attempt = (fn: () => Promise<unknown>) => {
    void fn().catch(() => setNotice("Session action failed; retry or close the session."));
  };
  return (
    <div className="log-viewer">
      <p className="detail-identity">
        {s.label} / {s.request.resource.namespace} / {s.request.resource.name} / {s.request.container} ·{" "}
        {s.request.previous ? "Previous" : "Current"} · UID {s.request.resource.uid} · generation {s.request.resource.connection.generation}
      </p>
      <div className="filter-bar">
        <strong>{s.status}</strong>
        <button disabled={!s.active} onClick={() => attempt(() => store.stop(s.id))}>
          Stop
        </button>
        <button onClick={() => store.clear(s.id)}>Clear view</button>
        <button
          disabled={s.active}
          onClick={() => {
            const changed =
              connection &&
              (connection.id !== s.request.resource.connection.id || connection.generation !== s.request.resource.connection.generation);
            if (
              window.confirm(
                `${changed ? "Confirm the newly connected target. " : ""}Resume validates the same Pod UID. A new segment can overlap or omit logs. Continue?`,
              )
            )
              attempt(() => store.resume(s.id, connection));
          }}
        >
          Resume
        </button>
        <button onClick={() => attempt(() => store.close(s.id))}>Close</button>
        <label>
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
      <div className="filter-bar">
        <input
          placeholder="Search retained text"
          aria-label="Search retained logs"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
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
      </div>
      <p className="muted">
        Server timestamps · {bytes} retained bytes · {s.dropped} dropped bytes · segment {s.segments} · display window ≤64 KiB
      </p>
      {(s.loss || s.error) && (
        <p className="warning" role="status">
          {s.error || s.loss}
        </p>
      )}
      <p role="status">{notice}</p>
      <pre className="log-text">{displayWindow(s.text, start) || "No retained log text."}</pre>
    </div>
  );
}
