/*
Purpose: Display a captured, read-only YAML representation.
Used by: Workload detail YAML sections.
Responsibilities: Toggle captured managed fields, search and copy exactly the displayed text.
Important: Literal values may be sensitive; nothing is exported or persisted automatically.
*/
import { useRef, useState } from "react";
import type { YAMLDocument } from "../workloads/model";
export function ResourceYaml({ data }: { data: YAMLDocument }) {
  const [managed, setManaged] = useState(false),
    [query, setQuery] = useState(""),
    [notice, setNotice] = useState("");
  const area = useRef<HTMLTextAreaElement>(null);
  const text = managed ? data.withManagedFields : data.withoutManagedFields;
  function search() {
    const start = area.current?.selectionEnd ?? 0;
    const found = text.toLowerCase().indexOf(query.toLowerCase(), start);
    const index = found < 0 ? text.toLowerCase().indexOf(query.toLowerCase()) : found;
    if (index >= 0) {
      area.current?.focus();
      area.current?.setSelectionRange(index, index + query.length);
    }
    setNotice(index < 0 ? "No match in displayed YAML" : "Match selected");
  }
  return (
    <div className="evidence-section">
      <p>
        Server representation · resourceVersion {data.resourceVersion} · {data.fetchedAt}
      </p>
      <p className="muted">
        May contain sensitive literal values. Managed fields {managed ? "included" : "omitted"}. Original comments and ordering are not
        preserved.
      </p>
      <div className="filter-bar">
        <label>
          <input type="checkbox" checked={managed} onChange={(e) => setManaged(e.target.checked)} /> Show managed fields
        </label>
        <input
          aria-label="Search displayed YAML"
          placeholder="Search displayed YAML"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <button disabled={!query} onClick={search}>
          Find next
        </button>
        <button
          onClick={() =>
            void navigator.clipboard
              .writeText(text)
              .then(() => setNotice("Displayed YAML copied"))
              .catch(() => setNotice("Copy unavailable; select text to copy"))
          }
        >
          Copy displayed YAML
        </button>
      </div>
      <p role="status">{notice}</p>
      <textarea ref={area} className="yaml-view" aria-label="Read-only server YAML" readOnly spellCheck={false} value={text} />
    </div>
  );
}
