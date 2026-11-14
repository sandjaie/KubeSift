/*
Purpose: Display a captured, read-only YAML representation.
Used by: Workload detail YAML sections.
Responsibilities: Search, toggle captured managed fields and copy the active source.
Important: Literal values may be sensitive. Nothing is exported automatically.
*/
import { useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { CopyTextButton } from "../ui/CopyTextButton";
import type { YAMLDocument } from "../workloads/model";
import { evidenceTime } from "./ResourceOverview";
import { yamlMatches, yamlMatchLimit } from "./yamlSearch";

export function ResourceYaml({ data, containsConfigData }: { data: YAMLDocument; containsConfigData: boolean }) {
  const [managed, setManaged] = useState(false);
  const [query, setQuery] = useState("");
  const [match, setMatch] = useState<{ text: string; query: string; index: number }>();
  const area = useRef<HTMLTextAreaElement>(null);
  const mark = useRef<HTMLElement>(null);
  const text = managed ? data.withManagedFields : data.withoutManagedFields;
  const matches = useMemo(() => yamlMatches(text, query), [text, query]);
  const total = Math.min(matches.length, yamlMatchLimit);
  const index = match?.text === text && match.query === query ? match.index : -1;
  const offset = index < 0 ? -1 : matches[index];
  // Limit highlighted DOM independently of the server's 2 MiB capture bound.
  const lines = useMemo(() => (text.length <= 65536 ? (text.match(/[^\n]*\n|[^\n]+$/g) ?? []) : []), [text]);
  const highlight = lines.length > 0 && lines.length <= 1000;
  useLayoutEffect(() => {
    if (offset < 0) return;
    mark.current?.scrollIntoView({ block: "nearest", inline: "nearest" });
    if (area.current) {
      area.current.focus();
      area.current.setSelectionRange(offset, offset + query.length);
    }
  }, [offset, query, text]);
  function move(backwards: boolean) {
    if (total) setMatch({ text, query, index: index < 0 ? (backwards ? total - 1 : 0) : (index + (backwards ? -1 : 1) + total) % total });
  }
  function searchKey(event: KeyboardEvent) {
    if (event.key === "Enter" && !event.nativeEvent.isComposing) {
      event.preventDefault();
      move(event.shiftKey);
    }
  }
  let start = 0;
  return (
    <div className="evidence-section yaml-evidence">
      <div className="evidence-toolbar yaml-toolbar">
        <input
          aria-label="Search displayed YAML"
          placeholder="Search displayed YAML"
          spellCheck={false}
          autoCorrect="off"
          autoCapitalize="none"
          autoComplete="off"
          value={query}
          onKeyDown={searchKey}
          onChange={(event) => {
            setQuery(event.target.value);
            setMatch(undefined);
          }}
        />
        <div className="evidence-toolbar" role="group" aria-label="YAML search navigation">
          <button disabled={!total} aria-label="Previous match" title="Previous match" onClick={() => move(true)}>
            <span aria-hidden="true">↑</span>
          </button>
          <button disabled={!total} aria-label="Next match" title="Next match" onClick={() => move(false)}>
            <span aria-hidden="true">↓</span>
          </button>
          {query && (
            <span className="muted" role="status">
              {total ? `${index < 0 ? "" : `${index + 1} of `}${total}${matches.length > total ? "+" : ""} matches` : "No matches"}
            </span>
          )}
        </div>
        <label className="yaml-managed">
          <input
            type="checkbox"
            checked={managed}
            onChange={(event) => {
              setManaged(event.target.checked);
              setMatch(undefined);
            }}
          />
          Show managed fields
        </label>
        <CopyTextButton label="Copy displayed YAML" copiedLabel="Displayed YAML copied" key={String(managed)} text={text} />
      </div>
      {containsConfigData && <p className="muted yaml-note">ConfigMap values can contain sensitive configuration. Check before sharing.</p>}
      {highlight ? (
        <pre className="yaml-view yaml-source" tabIndex={0} aria-label="Read-only server YAML">
          {lines.map((line, row) => {
            const from = Math.max(0, offset - start);
            const to = Math.min(line.length, offset + query.length - start);
            start += line.length;
            return (
              <span className="yaml-line" data-line={row + 1} key={row}>
                {offset >= 0 && to > from ? (
                  <>
                    {line.slice(0, from)}
                    <mark ref={from === 0 && offset < start - line.length ? undefined : mark}>{line.slice(from, to)}</mark>
                    {line.slice(to)}
                  </>
                ) : (
                  <span className={/^\s*(?:#|[\w.-]+:)/.test(line) ? "yaml-key" : undefined}>{line}</span>
                )}
              </span>
            );
          })}
        </pre>
      ) : (
        <>
          <p className="muted">Plain text view</p>
          <textarea
            ref={area}
            className="yaml-view"
            onKeyDown={searchKey}
            aria-label="Read-only server YAML"
            readOnly
            wrap="off"
            spellCheck={false}
            value={text}
          />
        </>
      )}
      <details className="muted">
        <summary>
          Captured {evidenceTime(data.fetchedAt)} · resourceVersion {data.resourceVersion || "Unknown"}
        </summary>
        <p>
          Exact capture {data.fetchedAt || "Unknown"}. Server representation with managed fields {managed ? "included" : "omitted"}.
          Original comments and ordering may differ. Fields unknown to this app’s Kubernetes types may be omitted.
        </p>
      </details>
    </div>
  );
}
