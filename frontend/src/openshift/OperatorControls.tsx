/*
Purpose: Review Classic OLM changes from an explicit install namespace.
Used by: The Operators workspace.
Responsibilities: Collect catalog identities, keep staged removal separate and show all reviewed impact.
Important: Lifecycle controls apply only to Classic resources and confirm by native token.
*/
import { useEffect, useState, useRef } from "react";
import type { APIResourceDescriptor, WorkflowItem, WorkflowQuery } from "../../bindings/internal/kube/models";
import type { OperationResult, OperatorRequest, WorkflowPreview } from "../../bindings/internal/operation/models";
import { api, message } from "../connections/api";
import { readRequest } from "../workloads/api";
import { Modal } from "../ui/ReviewDialog";
import type { SubmitWorkflow } from "./RouteAction";
export type OperatorAction = "install-operator" | "approve-plan" | "remove-subscription" | "remove-csv";
export function OperatorControls({
  item,
  target,
  desc,
  resources,
  submit,
  results,
}: {
  item?: WorkflowItem;
  target: WorkflowQuery;
  desc: APIResourceDescriptor;
  resources: APIResourceDescriptor[];
  submit: SubmitWorkflow;
  results: OperationResult[];
}) {
  const [action, open] = useState<OperatorAction>();
  const d = (k: string) => resources.find((r) => r.kind === k);
  const classic = desc.group === "operators.coreos.com";
  if (!classic) return null;
  const fields = item ? JSON.parse(item.fields) : {};
  const receipt =
    item &&
    results.find(
      (r) =>
        r.action === "remove-subscription" &&
        r.state === "accepted" &&
        r.installedCSV?.uid === item.resource.uid &&
        r.installedCSV.namespace === target.namespace &&
        r.installedCSV.connection.id === target.connection.id &&
        r.installedCSV.connection.generation === target.connection.generation,
    );
  return (
    <>
      {desc.kind === "Subscription" && (
        <>
          {desc.create && d("PackageManifest")?.get && d("OperatorGroup")?.list && d("CatalogSource")?.get && (
            <button onClick={() => open("install-operator")}>Install operator</button>
          )}
          {item && desc.get && (
            <>
              {d("InstallPlan")?.patch && fields.approval === "Manual" && fields.plan && (
                <button onClick={() => open("approve-plan")}>Review InstallPlan approval</button>
              )}
              {desc.delete && <button onClick={() => open("remove-subscription")}>Remove Subscription</button>}
            </>
          )}
        </>
      )}
      {desc.kind === "ClusterServiceVersion" && receipt && desc.delete && !fields.copied && (
        <button onClick={() => open("remove-csv")}>Remove installed CSV</button>
      )}
      {action && (
        <OperatorReview
          action={action}
          item={item}
          target={target}
          submit={submit}
          removalID={receipt?.operationID ?? ""}
          close={() => open(undefined)}
        />
      )}
    </>
  );
}
function OperatorReview({
  action,
  item,
  target,
  submit,
  removalID,
  close,
}: {
  action: OperatorAction;
  item?: WorkflowItem;
  target: WorkflowQuery;
  submit: SubmitWorkflow;
  removalID: string;
  close: () => void;
}) {
  const install = action === "install-operator";
  const [name, setName] = useState(install ? "" : item!.resource.name),
    [catalogNamespace, setNS] = useState(""),
    [catalog, setCatalog] = useState(""),
    [pkg, setPkg] = useState(""),
    [channel, setChannel] = useState("");
  const [p, set] = useState<WorkflowPreview>(),
    [error, fail] = useState(""),
    [ack, type] = useState(""),
    [impact, ackImpact] = useState(false),
    [busy, working] = useState(false);
  const token = useRef(""),
    alive = useRef(true),
    cancel = useRef<() => void>(() => {});
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      cancel.current();
      if (token.current) void api.RevokeOperator(action, token.current);
    };
  }, []);
  async function review() {
    working(true);
    fail("");
    const r: OperatorRequest = {
      action,
      target: { ...target, name, uid: install ? "" : item!.resource.uid },
      version: install ? "" : item!.version,
      catalogNamespace,
      catalog,
      package: pkg,
      channel,
      removalID,
    };
    const req = readRequest((id) => api.PrepareOperator(id, r));
    cancel.current = req.cancel;
    try {
      const v = await req.promise;
      if (alive.current) {
        token.current = v.token;
        set(v);
      } else void api.RevokeOperator(action, v.token);
    } catch (e) {
      if (alive.current) fail(message(e));
    } finally {
      if (alive.current) working(false);
    }
  }
  return (
    <Modal title="Review Operator change" onCancel={close}>
      <p>Install namespace · {target.namespace}</p>
      {error && <p role="alert">{error}</p>}
      {!p ? (
        <>
          {install && (
            <fieldset disabled={busy}>
              {(
                [
                  ["Subscription name", name, setName],
                  ["Catalog namespace", catalogNamespace, setNS],
                  ["Catalog source", catalog, setCatalog],
                  ["Package", pkg, setPkg],
                  ["Channel", channel, setChannel],
                ] as const
              ).map(([label, val, update]) => (
                <label key={label}>
                  {label}
                  <input value={val} onChange={(e) => update(e.target.value)} />
                </label>
              ))}
            </fieldset>
          )}
          <button disabled={busy || !name || (install && !(catalogNamespace && catalog && pkg && channel))} onClick={() => void review()}>
            Review impact
          </button>
        </>
      ) : (
        <>
          <p>
            {p.cluster} / {p.namespace} / {p.name}
          </p>
          <p>{p.impact}</p>
          {p.admitted && (
            <>
              <h3>Admitted Subscription</h3>
              <pre>{p.admitted}</pre>
            </>
          )}
          <pre>{p.before}</pre>
          <label>
            <input type="checkbox" checked={impact} onChange={(e) => ackImpact(e.target.checked)} /> I acknowledge the reviewed target
            namespaces and cluster-wide effects.
          </label>
          <label>
            Type {p.name} <input value={ack} onChange={(e) => type(e.target.value)} />
          </label>
          <button
            disabled={!impact || ack !== p.name}
            onClick={() => {
              if (!token.current) return;
              token.current = "";
              submit(action, p.token, ack);
              close();
            }}
          >
            Confirm {action}
          </button>
        </>
      )}
      <button onClick={close}>Cancel</button>
    </Modal>
  );
}
