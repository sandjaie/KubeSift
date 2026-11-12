/*
Purpose: Keep connection information and recovery actions in a compact disclosure.
Used by: The active workspace identity bar.
Responsibilities: Explain source, reconnect and disconnect while exposing urgent recovery controls.
Important: Callbacks remain bound to the owning tab; opening this panel never connects or reloads credentials.
*/
import { useEffect, useRef } from "react";
import type { ConnectionTab } from "./model";
import { ConnectionStatus } from "./ConnectionStatus";
import { visibleState } from "./state";

export function ConnectionControls({
  tab,
  onRetry,
  onCancel,
  onReview,
}: {
  tab: ConnectionTab;
  onRetry: () => void;
  onCancel: () => void;
  onReview: () => void;
}) {
  const disclosure = useRef<HTMLDetailsElement>(null);
  const connecting = tab.view?.state === "connecting";
  const recover = !connecting && (!!tab.error || tab.pollFailed || visibleState(tab.view) !== "connected");
  useEffect(() => {
    const outside = (event: PointerEvent) => {
      if (event.target instanceof Node && !disclosure.current?.contains(event.target) && disclosure.current) {
        disclosure.current.open = false;
      }
    };
    document.addEventListener("pointerdown", outside);
    return () => document.removeEventListener("pointerdown", outside);
  }, []);
  function run(action: () => void) {
    if (disclosure.current) disclosure.current.open = false;
    action();
  }
  return (
    <div className="connection-controls">
      {connecting ? <button onClick={onCancel}>Cancel connection</button> : recover && <button onClick={onRetry}>Retry</button>}
      {tab.view?.problem?.code === "trust_required" && <button onClick={onReview}>Review helper</button>}
      <details
        ref={disclosure}
        className="connection-disclosure"
        onBlur={(event) => {
          if (!event.currentTarget.contains(event.relatedTarget)) event.currentTarget.open = false;
        }}
        onKeyDown={(event) => {
          if (event.key === "Escape") {
            event.preventDefault();
            event.stopPropagation();
            event.currentTarget.open = false;
            event.currentTarget.querySelector("summary")?.focus();
          }
        }}
      >
        <summary aria-label="Connection details and actions">
          <ConnectionStatus view={tab.view} unknown={tab.pollFailed} /> <span aria-hidden="true">⌄</span>
        </summary>
        <div className="connection-popover">
          <h3>Connection</h3>
          <dl>
            <div>
              <dt>Context</dt>
              <dd>{tab.entry.context}</dd>
            </div>
            <div>
              <dt>Source</dt>
              <dd>{tab.entry.sourceDisplay}</dd>
            </div>
            {tab.view?.lastSuccessAt && (
              <div>
                <dt>Last successful read</dt>
                <dd>{tab.view.lastSuccessAt}</dd>
              </div>
            )}
          </dl>
          <p>Reconnect reloads your credentials. Disconnect stops this connection and closes its tab.</p>
          <div className="connection-buttons">
            {connecting ? (
              <button onClick={() => run(onCancel)}>Cancel connection</button>
            ) : (
              <>
                <button onClick={() => run(onRetry)}>Reconnect</button>
                <button onClick={() => run(onCancel)}>Disconnect</button>
              </>
            )}
          </div>
        </div>
      </details>
    </div>
  );
}
