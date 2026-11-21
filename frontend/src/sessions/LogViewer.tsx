/*
Purpose: Show one live, searchable log view with window expansion.
Used by: The visible Pod Logs tab.
Responsibilities: Scroll all retained text, find matches and pause scrolling while reading history.
Important: The store bounds text at 1 MiB; this view never opens a native reader.
*/
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { findLogMatch, type LogSession } from "./logStore";
import { logRows } from "./logRows";
export function LogViewer({ session: s }: { session: LogSession }) {
  const [paused, setPaused] = useState<LogSession | null>(null);
  const following = paused === null;
  const shown = paused ?? s;
  const rows = useMemo(() => logRows(shown.text), [shown.text]);
  const [top, setTop] = useState(0);
  const [height, setHeight] = useState(280);
  const [expanded, setExpanded] = useState(false);
  const [query, setQuery] = useState("");
  const [match, setMatch] = useState(-1);
  const [notice, setNotice] = useState("");
  const viewport = useRef<HTMLDivElement>(null);
  const expandButton = useRef<HTMLButtonElement>(null);
  useLayoutEffect(() => {
    const node = viewport.current;
    if (node && following) {
      node.scrollTop = node.scrollHeight;
      setTop(node.scrollTop);
    }
  }, [shown.text, following, expanded, height]);
  useEffect(() => {
    const node = viewport.current;
    if (!node) return;
    const observer = new ResizeObserver(() => setHeight(node.clientHeight));
    observer.observe(node);
    return () => observer.disconnect();
  }, []);
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
  function find(backwards: boolean) {
    if (!query) return;
    const next = findLogMatch(shown.text, query, match, backwards);
    setPaused(shown);
    setMatch(next);
    setNotice(next < 0 ? "No match in retained logs" : "Match in retained logs");
    if (next >= 0 && viewport.current) {
      const row = rows.findIndex((row) => next >= row.start && next < row.start + row.text.length);
      viewport.current.scrollTop = Math.max(0, row * 20 - height / 2);
      setTop(viewport.current.scrollTop);
    }
  }
  const first = Math.max(0, Math.floor(top / 20) - 5);
  const visible = rows.slice(first, first + Math.ceil(height / 20) + 10);
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
              setPaused(null);
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
          spellCheck={false}
          autoCorrect="off"
          autoCapitalize="none"
          autoComplete="off"
          aria-label="Search logs"
          placeholder="Search logs…"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setMatch(-1);
            setNotice("");
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.nativeEvent.isComposing) find(e.shiftKey);
          }}
        />
        <button disabled={!query} onClick={() => find(true)}>
          Previous match
        </button>
        <button disabled={!query} onClick={() => find(false)}>
          Next match
        </button>
      </div>
      {!following && <p className="muted">Reading retained history · Live collection continues. Jump to latest to update.</p>}
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
      <div
        ref={viewport}
        className="log-text"
        tabIndex={0}
        aria-label="Log output"
        onScroll={(e) => {
          const node = e.currentTarget;
          setTop(node.scrollTop);
          if (node.scrollHeight - node.scrollTop - node.clientHeight >= 8 && following) setPaused(s);
        }}
      >
        <div style={{ height: rows.length * 20, position: "relative", minWidth: "100%", width: "max-content" }}>
          <div style={{ paddingTop: first * 20 }}>
            {visible.map((row) => {
              const highlightStart = Math.max(0, match - row.start);
              const highlightEnd = Math.min(row.text.length, match + query.length - row.start);
              return (
                <div className="log-row" key={row.start}>
                  {match >= 0 && highlightEnd > highlightStart ? (
                    <>
                      {row.text.slice(0, highlightStart)}
                      <mark>{row.text.slice(highlightStart, highlightEnd)}</mark>
                      {row.text.slice(highlightEnd)}
                    </>
                  ) : (
                    row.text || " "
                  )}
                </div>
              );
            })}
          </div>
        </div>
        {!shown.text && (s.active ? "Waiting for logs…" : "No log output.")}
      </div>
      <p className="muted">
        Initial request: last {s.request.tailLines} lines · Up to 1 MiB retained. Older rotated logs may be unavailable.
      </p>
    </section>
  );
}
