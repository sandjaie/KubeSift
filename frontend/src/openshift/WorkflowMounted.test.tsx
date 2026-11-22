// @vitest-environment happy-dom
/*
Purpose: Exercise scoped OpenShift navigation and reviewed controls with a mounted workspace.
Used by: Workflow UI contract checks.
Responsibilities: Confirm controlled projections, explicit logs and revocation on navigation.
Important: Bridge responses are synthetic and never connected to a cluster.
*/
import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, expect, test, vi } from "vitest";
import { WorkflowWorkspace } from "./WorkflowWorkspace";
import type { WorkflowItem, APIResourceDescriptor, WorkflowQuery } from "../../bindings/internal/kube/models";
import type { LogSessionsController } from "../sessions/useLogSessions";
const api = vi.hoisted(() => ({
  ReadWorkflow: vi.fn(),
  ReadRouteService: vi.fn(),
  PrepareRoute: vi.fn(),
  RevokeRoute: vi.fn(),
  PrepareBuildAction: vi.fn(),
  RevokeBuildAction: vi.fn(),
  PrepareOperator: vi.fn(),
  RevokeOperator: vi.fn(),
}));
vi.mock("../workloads/api", () => ({
  readRequest: (fn: (id: string) => Promise<unknown>) => ({ promise: fn("fixture"), cancel: () => {} }),
}));
vi.mock("../connections/api", () => ({ api, message: String }));
afterEach(() => {
  vi.clearAllMocks();
  vi.unstubAllGlobals();
});
const connection = { id: "origin", generation: 2 };
async function mount(kind: string, fields: object) {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  const group = kind === "Route" ? "route.openshift.io" : kind === "Subscription" ? "operators.coreos.com" : "build.openshift.io";
  const descriptor = (k: string): APIResourceDescriptor => ({
    group,
    version: "v1",
    resource: k.toLowerCase() + "s",
    kind: k,
    namespaced: true,
    list: true,
    get: true,
    patch: true,
    delete: true,
    create: true,
    mode: "generic",
    reason: "",
    handle: k,
  });
  const item: WorkflowItem = {
    resource: { connection, namespace: "team", apiVersion: group + "/v1", kind, name: "demo", uid: "one" },
    version: "3",
    fields: JSON.stringify(fields),
  };
  api.ReadWorkflow.mockImplementation(async (_id: string, q: WorkflowQuery) => {
    expect(q.connection).toEqual(connection);
    expect(q.namespace).toBe("team");
    expect(q.projectUID).toBe("project-one");
    return { items: [item], next: "" };
  });
  const open = vi.fn(async () => {});
  const submit = vi.fn(),
    host = document.body.appendChild(document.createElement("div")),
    root = createRoot(host);
  await act(async () =>
    root.render(
      <WorkflowWorkspace
        connection={connection}
        namespace="team"
        projectUID="project-one"
        resources={
          kind === "Subscription"
            ? [
                descriptor(kind),
                descriptor("InstallPlan"),
                descriptor("OperatorGroup"),
                descriptor("PackageManifest"),
                descriptor("CatalogSource"),
              ]
            : [descriptor(kind)]
        }
        split={{ widthPercent: 45, heightPercent: 45, onResize: vi.fn(), onCommit: vi.fn() }}
        results={[]}
        openGeneric={vi.fn()}
        actions={{ "buildconfigs/instantiate": true, "builds/clone": true, "builds/log": true }}
        logs={{ store: { open }, sessions: [] } as unknown as LogSessionsController}
        submit={submit}
      />,
    ),
  );
  const click = async (name: string) => {
    const b = [...host.querySelectorAll("button")].find((b) => b.textContent === name);
    expect(b, name).toBeTruthy();
    await act(async () => b!.click());
  };
  const close = async () => {
    await act(async () => root.unmount());
    host.remove();
  };
  return { host, click, close, submit, open };
}
test("Route lookup opens the current Service, returns to the source and revokes its reviewed deletion", async () => {
  const v = await mount("Route", { spec: { to: { name: "backend" }, alternateBackends: [] }, status: { ingress: [] } });
  try {
    api.ReadRouteService.mockResolvedValue({ resource: { name: "backend", uid: "service-one" }, type: "ClusterIP", ports: ["web 80/TCP"] });
    api.PrepareRoute.mockResolvedValue({ cluster: "origin", token: "review", typed: true, name: "demo" });
    await v.click("demo");
    await v.click("backend");
    expect(v.host.textContent).toContain("web 80/TCP");
    await v.click("Back to Route");
    await v.click("Delete Route");
    await v.click("Review change");
    expect(v.host.querySelector("dialog")?.textContent).toContain("Type demo");
    await v.click("Cancel");
    expect(api.RevokeRoute).toHaveBeenCalledWith("delete-route", "review");
    expect(v.submit).not.toHaveBeenCalled();
  } finally {
    await v.close();
  }
});
test("Build logs open only on request and a pending cancel review revokes on departure", async () => {
  const v = await mount("Build", { phase: "Running", sourceType: "Git" });
  api.PrepareBuildAction.mockResolvedValue({ token: "build-token", name: "demo" });
  await v.click("demo");
  expect(v.open).not.toHaveBeenCalled();
  await v.click("Open logs");
  const signal = v.open.mock.calls[v.open.mock.calls.length - 1] as unknown as [unknown, string, string, AbortSignal];
  expect(signal[3].aborted).toBe(false);
  await v.click("Close logs");
  expect(signal[3].aborted).toBe(true);
  await v.click("Cancel build");
  await v.close();
  expect(api.RevokeBuildAction).toHaveBeenCalledWith("cancel-build", "build-token");
  expect(v.submit).not.toHaveBeenCalled();
});
test("Operator plan approval shows full impact and requires explicit acknowledgement", async () => {
  const v = await mount("Subscription", { approval: "Manual", plan: { name: "plan" } });
  try {
    api.PrepareOperator.mockResolvedValue({
      token: "plan-token",
      name: "plan",
      before: "cluster-wide ClusterRole demo-reader",
      typed: true,
    });
    await v.click("demo");
    await v.click("Review InstallPlan approval");
    await v.click("Review impact");
    expect(v.host.querySelector("dialog")?.textContent).toContain("cluster-wide ClusterRole demo-reader");
    const confirm = [...v.host.querySelectorAll("button")].find((b) => b.textContent === "Confirm approve-plan");
    expect(confirm?.disabled).toBe(true);
    await v.click("Cancel");
    expect(api.RevokeOperator).toHaveBeenCalledWith("approve-plan", "plan-token");
  } finally {
    await v.close();
  }
});
