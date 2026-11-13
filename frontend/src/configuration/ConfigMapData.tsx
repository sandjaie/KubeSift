/*
Purpose: Browse and copy one captured ConfigMap value as inert text.
Used by: ConfigMap Data detail tab.
Responsibilities: Bound rendered key rows and preserve empty text and canonical base64.
Important: Selection and values are held only while viewing the capture.
*/
import { useMemo, useState } from "react";
import type { ResourceDetail } from "../workloads/model";
import { overviewObject } from "../workspace/ResourceOverview";
import type { ConfigurationCapture } from "./model";

export function configMapKeys(data: ResourceDetail) {
  const o = overviewObject(data) as ConfigurationCapture;
  return [
    ...Object.entries(o.data ?? {}).map(([key, value]) => ({ key, value, type: "Text", bytes: new TextEncoder().encode(value).length })),
    ...Object.entries(o.binaryData ?? {}).map(([key, value]) => ({
      key,
      value,
      type: "Base64",
      bytes: Math.floor((value.replace(/=+$/, "").length * 3) / 4),
    })),
  ].sort((a, b) => a.key.localeCompare(b.key));
}

export function ConfigMapData({ data }: { data: ResourceDetail }) {
  const keys = useMemo(() => configMapKeys(data), [data]);
  const [filter, setFilter] = useState("");
  const [page, setPage] = useState(0);
  const [selected, setSelected] = useState<(typeof keys)[number]>();
  const [wrap, setWrap] = useState(false);
  const [feedback, setFeedback] = useState("");
  const matches = keys.filter(({ key }) => key.toLowerCase().includes(filter.toLowerCase()));
  const rows = matches.slice(page * 100, (page + 1) * 100);
  async function copy(value: string) {
    try {
      await navigator.clipboard.writeText(value);
      setFeedback("Copied");
    } catch {
      setFeedback("Copy failed");
    }
  }
  return (
    <div className="evidence-section">
      <p className="muted">Values may contain sensitive text. Binary values are shown as base64.</p>
      <input
        aria-label="Filter captured keys"
        placeholder="Filter captured keys…"
        spellCheck={false}
        autoCorrect="off"
        autoCapitalize="none"
        autoComplete="off"
        value={filter}
        onChange={(e) => {
          setFilter(e.target.value);
          setPage(0);
          setSelected(undefined);
          setFeedback("");
        }}
      />
      <p>
        {rows.length} shown / {matches.length} matching / {keys.length} captured
      </p>
      {rows.length > 0 && (
        <div className="evidence-table-wrap">
          <table className="evidence-table">
            <thead>
              <tr>
                <th>Key</th>
                <th>Type</th>
                <th>Bytes</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <tr key={`${row.type}/${row.key}`}>
                  <td>
                    <button
                      aria-pressed={selected === row}
                      className="network-value"
                      onClick={() => {
                        setSelected(row);
                        setFeedback("");
                      }}
                    >
                      {row.key}
                    </button>
                  </td>
                  <td>{row.type}</td>
                  <td>{row.bytes}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {matches.length > 100 && (
        <nav className="inventory-pagination" aria-label="Captured key pages">
          <button disabled={page === 0} onClick={() => setPage(page - 1)}>
            Previous
          </button>
          <button disabled={(page + 1) * 100 >= matches.length} onClick={() => setPage(page + 1)}>
            Next
          </button>
        </nav>
      )}
      {selected && (
        <section>
          <h3 className="network-value">
            {selected.key} · {selected.type}
          </h3>
          <label>
            <input type="checkbox" checked={wrap} onChange={(e) => setWrap(e.target.checked)} /> Wrap
          </label>{" "}
          <button onClick={() => void copy(selected.value)}>{selected.type === "Base64" ? "Copy base64" : "Copy value"}</button>
          <span role="status"> {feedback}</span>
          {selected.value === "" && <p>Empty value</p>}
          <pre className="configuration-value" style={{ whiteSpace: wrap ? "pre-wrap" : "pre" }}>
            {selected.value}
          </pre>
        </section>
      )}
    </div>
  );
}
