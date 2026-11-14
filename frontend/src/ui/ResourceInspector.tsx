/*
Purpose: Inspect nested fields in a Kubernetes capture.
Used by: Structured fields and metadata in resource Overview sections.
Responsibilities: Show field labels, exact text values and lazily paged branches.
Important: Captured values remain inert text and the inspector never fetches more data.
*/
import { useState } from "react";
import { EvidencePage } from "./EvidencePage";

function scalarText(value: unknown) {
  return value == null ? "Not reported" : value === "" ? '""' : String(value);
}

function Node({ value, label, depth, empty }: { value: unknown; label?: string; depth: number; empty: string }) {
  const [open, setOpen] = useState(false);
  const entries = value != null && typeof value === "object" ? Object.entries(value) : undefined;
  const array = Array.isArray(value);
  const unit = array ? "item" : "field";
  const longText = label && typeof value === "string" && value.length > 180 ? value : undefined;
  const indent = { paddingInlineStart: Math.min(depth, 8) * 10 };
  if (!entries?.length && longText === undefined) {
    const text = entries ? (depth ? (array ? "[]" : "{}") : empty) : scalarText(value);
    return (
      <div className="inspector-leaf" style={indent}>
        {label && <span className="inspector-key">{label}</span>}
        <span className="inspector-value">{text}</span>
      </div>
    );
  }
  return (
    <details
      onToggle={(event) => {
        if (event.target === event.currentTarget) setOpen(event.currentTarget.open);
      }}
    >
      <summary style={indent}>
        {label && <span className="inspector-key-text">{label} </span>}
        <span className={label ? "inspector-count" : undefined}>
          {entries ? `${entries.length} ${unit}${entries.length === 1 ? "" : "s"}` : "Long text"}
        </span>
      </summary>
      {open &&
        (entries ? (
          <EvidencePage
            items={entries}
            render={([key, item]) => (
              <Node key={key} label={array ? `[${key}]` : key || '""'} value={item} depth={depth + 1} empty={empty} />
            )}
          />
        ) : (
          <div className="inspector-leaf" style={indent}>
            <pre className="inspector-text inspector-value" tabIndex={0}>
              {longText}
            </pre>
          </div>
        ))}
    </details>
  );
}

export function ResourceInspector({ value, label, empty = "None captured" }: { value: unknown; label?: string; empty?: string }) {
  if (!label && (value == null || typeof value !== "object")) return scalarText(value);
  return (
    <div className="resource-inspector">
      <Node value={value} label={label} depth={0} empty={empty} />
    </div>
  );
}
