/*
Purpose: Request Build actions and bounded logs from a captured Build or BuildConfig.
Used by: The OpenShift Builds workspace.
Responsibilities: Keep imperative actions separate from explicitly opened log sessions.
Important: Unmount cancels pending reads, revokes review and closes the stream.
*/
import { useState, useRef, useEffect } from "react";
import { api, message } from "../connections/api";
import type { WorkflowItem, WorkflowQuery, APIResourceDescriptor } from "../../bindings/internal/kube/models";
import type { WorkflowPreview } from "../../bindings/internal/operation/models";
import type { SubmitWorkflow } from "./RouteAction";
import type { LogSessionsController } from "../sessions/useLogSessions";
import { LogViewer } from "../sessions/LogViewer";
import { Modal } from "../ui/ReviewDialog";
import { readRequest } from "../workloads/api";
export type BuildAction = "start-build" | "rebuild" | "cancel-build";
export function BuildControls({
  item,
  target,
  actions,
  desc,
  logs,
  submit,
}: {
  item: WorkflowItem;
  target: WorkflowQuery;
  actions: Record<string, boolean | undefined>;
  desc: APIResourceDescriptor;
  logs: LogSessionsController;
  submit: SubmitWorkflow;
}) {
  const [action, setAction] = useState<BuildAction>(),
    [stream, setStream] = useState(false),
    [follow, setFollow] = useState(true),
    [error, fail] = useState("");
  const fields = JSON.parse(item.fields),
    build = desc.kind === "Build";
  const tab = JSON.stringify([item.resource.connection, item.resource.namespace, item.resource.uid, target.projectUID]);
  useEffect(() => {
    if (!stream) return;
    const view = new AbortController();
    fail("");
    void logs.store
      .open(
        { resource: item.resource, projectUID: target.projectUID, container: "", previous: false, follow, tailLines: 200 },
        tab,
        item.resource.name,
        view.signal,
      )
      .catch((e) => {
        if (!view.signal.aborted) fail(message(e));
      });
    return () => view.abort();
  }, [logs.store, tab, stream, follow, item.resource, target.projectUID]);
  return (
    <>
      {!build && actions["buildconfigs/instantiate"] && (
        <button disabled={fields.sourceType === "Binary"} onClick={() => setAction("start-build")}>
          Start build
        </button>
      )}
      {build && actions["builds/clone"] && (
        <button disabled={fields.sourceType === "Binary"} onClick={() => setAction("rebuild")}>
          Rebuild
        </button>
      )}
      {build && desc.patch && (
        <button
          disabled={fields.cancelled || !["New", "Pending", "Running"].includes(fields.phase)}
          onClick={() => setAction("cancel-build")}
        >
          Cancel build
        </button>
      )}
      {build && actions["builds/log"] && (
        <>
          <label>
            <input type="checkbox" checked={follow} onChange={(e) => setFollow(e.target.checked)} /> Follow
          </label>
          <button onClick={() => setStream(!stream)}>{stream ? "Close logs" : "Open logs"}</button>
        </>
      )}
      {error && <p role="alert">{error}</p>}
      {stream &&
        logs.sessions
          .filter((s) => s.tab === tab)
          .slice(-1)
          .map((s) => <LogViewer key={s.id} session={s} />)}
      {action && <BuildReview action={action} item={item} target={target} submit={submit} close={() => setAction(undefined)} />}
    </>
  );
}
function BuildReview({
  action,
  item,
  target,
  submit,
  close,
}: {
  action: BuildAction;
  item: WorkflowItem;
  target: WorkflowQuery;
  submit: SubmitWorkflow;
  close: () => void;
}) {
  const [p, set] = useState<WorkflowPreview>(),
    [error, fail] = useState(""),
    [ack, type] = useState("");
  const token = useRef("");
  useEffect(() => {
    let current = true;
    const req = readRequest((id) =>
      api.PrepareBuildAction(id, {
        target: { ...target, name: item.resource.name, uid: item.resource.uid },
        version: item.version,
        action,
      }),
    );
    void req.promise.then(
      (value) => {
        if (current) {
          token.current = value.token;
          set(value);
        } else void api.RevokeBuildAction(action, value.token);
      },
      (e) => {
        if (current) fail(message(e));
      },
    );
    return () => {
      current = false;
      req.cancel();
      if (token.current) void api.RevokeBuildAction(action, token.current);
    };
  }, []);
  return (
    <Modal title="Review Build action" onCancel={close}>
      {error && <p role="alert">{error}</p>}
      {p && (
        <>
          <p>
            {p.cluster} / {p.namespace} / {p.name}
          </p>
          <p>{p.impact}</p>
          <pre>{p.before}</pre>
          {p.typed && (
            <label>
              Type {p.name}
              <input value={ack} onChange={(e) => type(e.target.value)} />
            </label>
          )}
          <button
            disabled={p.typed && ack !== p.name}
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
      <button onClick={close}>Close</button>
    </Modal>
  );
}
