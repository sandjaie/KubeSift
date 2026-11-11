/*
Purpose: Let users find and explicitly open configured Kubernetes contexts.
Used by: Product entry and cluster selector.
Responsibilities: Distinguish source groups and show safe catalog failures without connecting at startup.
Important: Configuration actions belong in Settings, never inline credential forms.
*/
import { useState } from "react";
import type { CatalogEntry, SourceResult } from "./api";
import type { ConnectionTab } from "./model";
import { tabKey } from "./state";
import { ConnectionStatus } from "./ConnectionStatus";
export function ClusterList({
  sources,
  connections,
  onSelect,
  onSettings,
  loading,
  error,
}: {
  sources: SourceResult[];
  connections: Record<string, ConnectionTab>;
  onSelect: (entry: CatalogEntry) => void;
  onSettings: () => void;
  loading: boolean;
  error: string;
}) {
  const [query, setQuery] = useState("");
  const entries = sources
    .flatMap((s) => s.contexts ?? [])
    .sort((a, b) => a.context.localeCompare(b.context) || a.sourceDisplay.localeCompare(b.sourceDisplay))
    .filter((c) => `${c.context} ${c.sourceDisplay} ${c.environment}`.toLowerCase().includes(query.toLowerCase()));
  return (
    <main className="cluster-list">
      <div className="section-heading">
        <div>
          <h1>Clusters</h1>
          <p className="muted">Choose a context to connect. Credentials come from your kubeconfig.</p>
        </div>
        <button onClick={onSettings}>Configure sources</button>
      </div>
      <input
        aria-label="Search clusters"
        placeholder="Search contexts, sources or labels…"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
      />
      {error && <p role="alert">{error}</p>}
      {sources
        .filter((s) => s.problem)
        .map((s) => (
          <p className="settings-error" role="alert" key={s.sourceId}>
            {s.path || s.sourceId}: {s.problem!.message} <button onClick={onSettings}>Open Settings</button>
          </p>
        ))}
      {loading ? (
        <p>Loading local sources…</p>
      ) : entries.length === 0 ? (
        <p>{query ? "No matching contexts." : "No contexts available. Add or restore a source in Settings."}</p>
      ) : (
        <div className="cluster-cards">
          {entries.map((entry) => {
            const t = connections[tabKey(entry)];
            return (
              <button className="cluster-card" key={tabKey(entry)} onClick={() => onSelect(entry)}>
                <span className="section-heading">
                  <strong>{entry.context}</strong>
                  <ConnectionStatus view={t?.view ?? null} unknown={t?.pollFailed} />
                </span>
                <span>
                  Namespace: {entry.namespace}
                  {entry.environment && ` · ${entry.environment}`}
                </span>
                <span className="muted">{entry.endpoint}</span>
                <span className="source-path">{entry.sourceDisplay}</span>
                {entry.labelNeedsReview && <span className="warning">Environment label needs review in Settings</span>}
              </button>
            );
          })}
        </div>
      )}
    </main>
  );
}
