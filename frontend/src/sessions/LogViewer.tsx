/*
Purpose: Show one live, searchable log view with window expansion.
Used by: The visible Pod Logs tab.
Responsibilities: Scroll all retained text, find matches and pause scrolling while reading history.
Important: The store bounds text at 1 MiB; this view never opens a native reader.
*/
import { useEffect, useRef, useState } from "react";
import { findLogMatch, scrollLogViewport, type LogSession } from "./logStore";
export function LogViewer({ session: s }: { session: LogSession }) {
  const [following, setFollowing] = useState(true);
  const [expanded, setExpanded] = useState(false);
  const [query, setQuery] = useState("");
  const [match, setMatch] = useState(-1);
  const [notice, setNotice] = useState("");
  const viewport = useRef<HTMLPreElement>(null);
  const selected = useRef<HTMLElement>(null);
  const expandButton = useRef<HTMLButtonElement>(null);
  useEffect(() => scrollLogViewport(viewport.current, following), [s.text, following, expanded]);
  useEffect(() => {
    if (match >= 0) selected.current?.scrollIntoView({ block: "center" });
  }, [match, query]);
  useEffect(() => {
    if (!expanded) return;
    expandButton.current?.focus();
    const escape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setExpanded(false);
        expandButton.current?.focus();
      }
    };
    window.addEventListener("keydown", escape);
    return () => window.removeEventListener("keydown", escape);
  }, [expanded]);
  // Retention eviction changes offsets, so an old search selection must be discarded.
  useEffect(() => {
    setMatch(-1);
  }, [s.dropped]);
  function find(backwards: boolean) {
    if (!query) return;
    const next = findLogMatch(s.text, query, match, backwards);
    setFollowing(false);
    setMatch(next);
    setNotice(next < 0 ? "No match in retained logs" : "Match in retained logs");
    if (next === match) selected.current?.scrollIntoView({ block: "center" });
  }
  return (
    <section
      className={`log-viewer${expanded ? " log-expanded" : ""}`}
      aria-label="Container logs"
      role={expanded ? "dialog" : undefined}
      aria-modal={expanded || undefined}
      onKeyDown={(e) => {
        if (!expanded || e.key !== "Tab") return;
        const nodes = Array.from(e.currentTarget.querySelectorAll<HTMLElement>('button:not(:disabled),input,[tabindex="0"]'));
        const first = nodes[0],
          last = nodes[nodes.length - 1];
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault();
          last?.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first?.focus();
        }
      }}
    >
      <div className="log-toolbar">
        <strong>
          {s.request.container} · {s.request.previous ? "Previous" : "Current"}
        </strong>
        <span className="log-status">
          {s.status === "complete" ? "Loaded" : s.status === "running" ? (s.request.follow ? "Live" : "Loading") : s.status}
        </span>
        <button ref={expandButton} aria-expanded={expanded} onClick={() => setExpanded(!expanded)}>
          {expanded ? "Restore size" : "Expand to window"}
        </button>
        {!following && (
          <button
            onClick={() => {
              setMatch(-1);
              setFollowing(true);
            }}
          >
            Jump to latest
          </button>
        )}
      </div>
      {expanded && (
        <p className="log-target">
          {s.label} · {s.request.resource.namespace} · {s.request.resource.name}
        </p>
      )}
      <div className="log-toolbar">
        <input
          aria-label="Search logs"
          placeholder="Search logs…"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setMatch(-1);
            setNotice("");
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter") find(e.shiftKey);
          }}
        />
        <button disabled={!query} onClick={() => find(true)}>
          Previous match
        </button>
        <button disabled={!query} onClick={() => find(false)}>
          Next match
        </button>
      </div>
      {notice && <p role="status">{notice}</p>}
      {s.dropped > 0 && (
        <p className="warning" role="status">
          Earlier output was discarded at the retention limit ({s.dropped.toLocaleString()} bytes omitted).
        </p>
      )}
      {(s.loss || s.error) && (
        <p className="warning" role="status">
          {s.error || s.loss}
        </p>
      )}
      <pre
        ref={viewport}
        className="log-text"
        tabIndex={0}
        aria-label="Log output"
        onScroll={(e) => {
          const node = e.currentTarget;
          setFollowing(node.scrollHeight - node.scrollTop - node.clientHeight < 8);
        }}
      >
        {match >= 0 ? (
          <>
            {s.text.slice(0, match)}
            <mark ref={selected}>{s.text.slice(match, match + query.length)}</mark>
            {s.text.slice(match + query.length)}
          </>
        ) : (
          s.text || (s.active ? "Waiting for logs…" : "No log output.")
        )}
      </pre>
      <p className="muted">
        Initial request: last {s.request.tailLines} lines · Up to 1 MiB retained. Older rotated logs may be unavailable.
      </p>
    </section>
  );
}
