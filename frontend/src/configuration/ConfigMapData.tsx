/*
Purpose: Browse and copy captured ConfigMap values as inert text.
Used by: ConfigMap Data detail tab.
Responsibilities: Bound rendered key rows and preserve empty text and canonical base64.
Important: Values are held only while viewing the capture.
*/
import { useMemo, useState } from "react";
import { Disclosure } from "../access-control/PolicyRules";
import { CopyTextButton } from "../ui/CopyTextButton";
import { EvidencePage } from "../ui/EvidencePage";
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
  const matches = keys.filter(({ key }) => key.toLowerCase().includes(filter.toLowerCase()));
  return (
    <div className="evidence-section">
      <div className="evidence-toolbar config-map-toolbar">
        <input
          aria-label="Filter captured keys"
          placeholder="Filter captured keys…"
          spellCheck={false}
          autoCorrect="off"
          autoCapitalize="none"
          autoComplete="off"
          value={filter}
          onChange={(event) => setFilter(event.target.value)}
        />
        <span className="muted" role="status">
          {matches.length === keys.length ? keys.length : `${matches.length} of ${keys.length}`} {keys.length === 1 ? "key" : "keys"}
        </span>
      </div>
      {keys.length > 0 && <p className="muted yaml-note">Values can contain sensitive configuration. Check before sharing.</p>}
      {!matches.length && <p>{keys.length ? "No keys match this filter." : "No keys captured."}</p>}
      {matches.length > 0 && (
        <EvidencePage
          key={filter}
          items={matches}
          wrap={(rows) => <ul className="config-map-entries">{rows}</ul>}
          render={({ key, type, bytes, value }) => {
            const label = type === "Base64" ? "Copy base64" : "Copy value";
            const long = value.length > 120 || value.includes("\n");
            return (
              <li key={`${type}/${key}`}>
                <div className="config-map-entry-heading">
                  <code>{key}</code>
                  <span className="muted">
                    {type} · {bytes} {bytes === 1 ? "byte" : "bytes"}
                  </span>
                  <CopyTextButton label={`${label} for ${key}`} copiedLabel={`Copied ${key}`} text={value} />
                </div>
                {long ? (
                  <Disclosure label={`View ${key}`}>
                    <pre className="configuration-value" tabIndex={0}>
                      {value}
                    </pre>
                  </Disclosure>
                ) : value === "" ? (
                  <span className="muted">Empty value</span>
                ) : (
                  <pre className="configuration-value">{value}</pre>
                )}
              </li>
            );
          }}
        />
      )}
    </div>
  );
}
