/*
Purpose: Review one field-limited Route change for the captured namespace.
Used by: OpenShift Routes workspace.
Responsibilities: Freeze the draft during review and revoke its token when dismissed.
Important: Confirmation only submits the reviewed token and acknowledgement.
*/
import { useEffect, useRef, useState } from "react";
import { Modal } from "../ui/ReviewDialog";
import { api, message } from "../connections/api";
import { readRequest } from "../workloads/api";
import type { WorkflowQuery, WorkflowItem, RouteFields } from "../../bindings/internal/kube/models";
import type { WorkflowPreview } from "../../bindings/internal/operation/models";
import type { OperationAction } from "../operations/api";
export type RouteActionName = "create-route" | "edit-route" | "delete-route";
export type SubmitWorkflow = (action: OperationAction, token: string, ack: string) => void;
export function RouteAction({
  action,
  target,
  item,
  close,
  submit,
}: {
  action: RouteActionName;
  target: WorkflowQuery;
  item?: WorkflowItem;
  close: () => void;
  submit: SubmitWorkflow;
}) {
  const create = action === "create-route",
    remove = action === "delete-route";
  const original = !create && item ? JSON.parse(item.fields).spec : { to: { name: "", kind: "Service" } };
  const [name, setName] = useState(create ? "" : (item?.resource.name ?? "")),
    [host, setHost] = useState(original.host ?? ""),
    [path, setPath] = useState(original.path ?? ""),
    [service, setService] = useState(original.to.name),
    [port, setPort] = useState(JSON.stringify(original.port?.targetPort) ?? ""),
    [weight, setWeight] = useState(original.to.weight?.toString() ?? ""),
    [alts, setAlts] = useState(JSON.stringify(original.alternateBackends ?? [])),
    [tls, setTls] = useState(original.tls?.termination ?? "");
  const [preview, setPreview] = useState<WorkflowPreview>(),
    [busy, setBusy] = useState(false),
    [error, fail] = useState(""),
    [ack, setAck] = useState("");
  const lifetime = useRef(0),
    token = useRef(""),
    cancel = useRef<() => void>(() => {});
  function revoke() {
    if (token.current) {
      void api.RevokeRoute(action, token.current);
      token.current = "";
    }
  }
  useEffect(
    () => () => {
      lifetime.current++;
      cancel.current();
      revoke();
    },
    [],
  );
  async function review() {
    const n = lifetime.current;
    setBusy(true);
    fail("");
    try {
      const fields: RouteFields = {
        ...original,
        host,
        path,
        to: { ...original.to, name: service, weight: weight === "" ? undefined : Number(weight) },
        port: port ? { targetPort: JSON.parse(port) } : undefined,
        alternateBackends: JSON.parse(alts),
        tls: create ? (tls ? { termination: tls } : undefined) : original.tls,
      };
      const req = readRequest((id) =>
        api.PrepareRoute(id, {
          target: { ...target, name, uid: create ? "" : item!.resource.uid },
          version: create ? "" : item!.version,
          action,
          fields,
        }),
      );
      cancel.current = req.cancel;
      const p = await req.promise;
      if (n !== lifetime.current) {
        void api.RevokeRoute(action, p.token);
        return;
      }
      token.current = p.token;
      setPreview(p);
    } catch (e) {
      if (n === lifetime.current) fail(message(e));
    } finally {
      if (n === lifetime.current) setBusy(false);
    }
  }
  return (
    <Modal title={create ? "Create Route" : remove ? "Delete Route" : "Edit Route"} onCancel={close}>
      <p>
        {target.namespace} / {name}
      </p>
      {error && <p role="alert">{error}</p>}
      {!preview ? (
        <>
          <fieldset disabled={busy}>
            {create && (
              <>
                <label>
                  Name{" "}
                  <input
                    autoCapitalize="none"
                    autoCorrect="off"
                    spellCheck={false}
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                  />
                </label>
                <label>
                  Host{" "}
                  <input
                    autoCapitalize="none"
                    autoCorrect="off"
                    spellCheck={false}
                    value={host}
                    onChange={(e) => setHost(e.target.value)}
                  />
                </label>
                <label>
                  TLS{" "}
                  <select value={tls} onChange={(e) => setTls(e.target.value)}>
                    <option value="">Cleartext</option>
                    <option>edge</option>
                    <option>passthrough</option>
                  </select>
                </label>
              </>
            )}
            {!remove && (
              <>
                <label>
                  Path{" "}
                  <input
                    autoCapitalize="none"
                    autoCorrect="off"
                    spellCheck={false}
                    value={path}
                    onChange={(e) => setPath(e.target.value)}
                  />
                </label>
                <label>
                  Primary Service{" "}
                  <input
                    autoCapitalize="none"
                    autoCorrect="off"
                    spellCheck={false}
                    value={service}
                    onChange={(e) => setService(e.target.value)}
                  />
                </label>
                <label>
                  Weight <input type="number" min="0" max="256" value={weight} onChange={(e) => setWeight(e.target.value)} />
                </label>
                <label>
                  Target port (number or quoted name){" "}
                  <input
                    autoCapitalize="none"
                    autoCorrect="off"
                    spellCheck={false}
                    value={port}
                    onChange={(e) => setPort(e.target.value)}
                  />
                </label>
                <label>
                  Alternate Services (JSON array) <textarea value={alts} onChange={(e) => setAlts(e.target.value)} />
                </label>
              </>
            )}
          </fieldset>
          <button disabled={busy || !name || (!remove && !service)} onClick={() => void review()}>
            Review change
          </button>
        </>
      ) : (
        <>
          <p>
            {preview.cluster} · {preview.impact}
          </p>
          {preview.before && (
            <details>
              <summary>Current Route</summary>
              <pre>{preview.before}</pre>
            </details>
          )}
          {preview.admitted && (
            <>
              <h3>Server dry-run</h3>
              <pre>{preview.admitted}</pre>
            </>
          )}
          {preview.typed && (
            <label>
              Type {name}{" "}
              <input autoCapitalize="none" autoCorrect="off" spellCheck={false} value={ack} onChange={(e) => setAck(e.target.value)} />
            </label>
          )}
          <button
            onClick={() => {
              revoke();
              setPreview(undefined);
              setAck("");
            }}
          >
            Back
          </button>
          <button
            disabled={preview.typed && ack !== name}
            onClick={() => {
              if (!token.current) return;
              const saved = token.current;
              token.current = "";
              submit(action, saved, ack);
              close();
            }}
          >
            Confirm {remove ? "deletion" : "change"}
          </button>
        </>
      )}
      <button onClick={close}>Cancel</button>
    </Modal>
  );
}
